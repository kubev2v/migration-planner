package service

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/kubev2v/migration-planner/internal/config"

	"golang.org/x/sync/singleflight"

	"errors"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/image"
	"github.com/kubev2v/migration-planner/pkg/metrics"
	"github.com/kubev2v/migration-planner/pkg/version"
	"go.uber.org/zap"

	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	// imageExpirationTime define the expiration of the image download URL
	imageExpirationTime = 4 * time.Hour
	estimatedImageSize  = 3 * 1024 * 1024 * 1024 // 3 GiB
	tokenEtagSep        = "+"                    // valid jwt and sha256 shouldn't contain '+'
)

type fileInfo struct {
	path string
	mod  time.Time
}
type ImageSvc struct {
	store         store.Store
	tempImagesDir string
	dirLimit      string
	baseUrl       string
	g             singleflight.Group
}

func NewImageSvc(s store.Store, config *config.Config) *ImageSvc {
	return &ImageSvc{
		store:         s,
		tempImagesDir: config.Service.TempImagesDir,
		dirLimit:      config.Service.TempImagesDirLimit,
		baseUrl:       config.Service.BaseImageEndpointUrl,
	}
}

func (i *ImageSvc) FilePath(etag string) string {
	return filepath.Join(i.tempImagesDir, fmt.Sprintf("%s.ova", etag))
}

func (i *ImageSvc) GenerateDownloadURL(ctx context.Context, id uuid.UUID) (string, time.Time, error) {
	source, err := i.getSource(ctx, id.String())
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return "", time.Time{}, NewErrSourceNotFound(id)
		}
		return "", time.Time{}, fmt.Errorf("failed to get source %s: %w", id, err)
	}

	etag, err := i.generateOVA(ctx, source)
	if err != nil {
		return "", time.Time{}, err
	}

	url, expireAt, err := i.downloadURL(source, etag)
	if err != nil {
		return "", time.Time{}, err
	}

	return url, expireAt, err
}

func (i *ImageSvc) ValidateToken(ctx context.Context, token string) error {
	parsedToken, err := jwt.Parse(token, func(t *jwt.Token) (interface{}, error) {
		return i.getSourceKey(ctx, t)
	})
	if err != nil {
		return fmt.Errorf("unauthorized: %v", err)
	}

	return parsedToken.Claims.Valid()
}

func (i *ImageSvc) Validate(ctx context.Context, sourceId string) error {
	if _, err := i.getSource(ctx, sourceId); err != nil {
		return err
	}

	return nil
}

func (i *ImageSvc) UpdateAgentVersion(sourceId string) {
	versionInfo := version.Get()
	if !version.IsValidAgentVersion(versionInfo.AgentVersionName) {
		zap.S().Named("image_service").Warnw("agent version not valid, skipping storage", "source_id", sourceId, "agent_version_name", versionInfo.AgentVersionName, "agent_git_commit", versionInfo.AgentGitCommit)
		return
	}

	// Use detached context to ensure version persists even if client disconnects
	persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use atomic update to prevent race conditions during concurrent downloads
	if err := i.store.ImageInfra().UpdateAgentVersion(persistCtx, sourceId, versionInfo.AgentVersionName); err != nil {
		zap.S().Named("image_service").Warnw("failed to update agent version", "error", err, "source_id", sourceId, "agent_version", versionInfo.AgentVersionName)
		return
	}

	zap.S().Named("image_service").Infow("stored agent version", "source_id", sourceId, "agent_version", versionInfo.AgentVersionName)
}

func (i *ImageSvc) ParseTokenETag(input string) (string, string, error) {
	var token, etag string

	parts := strings.Split(input, tokenEtagSep)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid format")
	}

	token = parts[0]
	etag = parts[1]

	if token == "" || etag == "" {
		return "", "", fmt.Errorf("missing token or etag")
	}

	return token, etag, nil
}

func (i *ImageSvc) buildTokenETag(token, etag string) string {
	return token + tokenEtagSep + etag
}

func (i *ImageSvc) cleanOldFiles(files ...fileInfo) error {
	if len(files) == 0 {
		return nil
	}

	var errs []error

	for _, f := range files {
		if err := os.Remove(f.path); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

func (i *ImageSvc) listFilesSplitByAge(cutoff time.Duration) ([]fileInfo, []fileInfo, int64, error) {
	entries, err := os.ReadDir(i.tempImagesDir)
	if err != nil {
		return nil, nil, 0, err
	}

	files := make([]fileInfo, 0, len(entries))
	var totalSize int64

	threshold := time.Now().Add(-cutoff)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return nil, nil, 0, err
		}

		totalSize += info.Size()
		files = append(files, fileInfo{
			path: filepath.Join(i.tempImagesDir, entry.Name()),
			mod:  info.ModTime(),
		})
	}

	// Sort by oldest first
	sort.Slice(files, func(a, b int) bool {
		return files[a].mod.Before(files[b].mod)
	})

	// Find the split point
	// Everything BEFORE this index is older than the threshold
	splitIdx := sort.Search(len(files), func(j int) bool {
		return files[j].mod.After(threshold)
	})

	older := files[:splitIdx]
	newer := files[splitIdx:]

	return older, newer, totalSize, nil
}

func (i *ImageSvc) ensureEnoughSpace() error {
	staleFiles, activeFiles, dirSize, err := i.listFilesSplitByAge(imageExpirationTime)
	if err != nil {
		return err
	}

	if err := i.cleanOldFiles(staleFiles...); err != nil {
		return err
	}

	ok, err := i.hasSpace(estimatedImageSize, dirSize)
	if err != nil {
		return err
	}

	if ok {
		return nil
	}

	// Decision: Delete the oldest file in the dir

	if len(activeFiles) > 0 {
		return os.Remove(activeFiles[0].path)
	}

	return fmt.Errorf("no files in directory. The directory is too small")
}

func (i *ImageSvc) hasSpace(required uint64, directorySize int64) (bool, error) {
	// Optional cap (Kubernetes quantity, e.g. "10Gi"). Empty means no limit from config.
	if strings.TrimSpace(i.dirLimit) != "" {
		q, err := resource.ParseQuantity(i.dirLimit)
		if err != nil {
			return false, err
		}
		limitBytes := q.Value()
		if directorySize+int64(required) > limitBytes {
			return false, nil
		}
	}

	// Check real filesystem space
	var stat syscall.Statfs_t
	if err := syscall.Statfs(i.tempImagesDir, &stat); err != nil {
		return false, err
	}

	available := stat.Bavail * uint64(stat.Bsize)

	return available > required, nil
}

func (i *ImageSvc) generateOVA(ctx context.Context, source *model.Source) (string, error) {
	b := image.NewImageBuilder(source.ImageInfra.SourceID).WithImageInfra(source.ImageInfra)
	etag, err := b.Etag()
	if err != nil {
		return "", err
	}

	tmpfile := i.FilePath(etag)
	_, err, _ = i.g.Do(etag, func() (interface{}, error) {

		if _, err := os.Stat(tmpfile); err == nil {
			return tmpfile, nil
		}

		if err := i.ensureEnoughSpace(); err != nil {
			return nil, err
		}

		f, err := os.Create(tmpfile)
		if err != nil {
			return nil, err
		}

		removeFile := false
		defer func() {
			_ = f.Close()
			if removeFile {
				_ = os.Remove(tmpfile)
			}
		}()

		token, err := i.generateAgentToken(ctx, source)
		if err != nil {
			removeFile = true
			return nil, err
		}

		if err := b.WithAgentToken(token).Generate(f); err != nil {
			removeFile = true
			metrics.IncreaseOvaDownloadsTotalMetric("failed")
			return nil, err
		}

		metrics.IncreaseOvaDownloadsTotalMetric("successful")
		return tmpfile, nil
	})

	if err != nil {
		return "", err
	}

	return etag, nil
}

func (i *ImageSvc) getSource(ctx context.Context, sourceId string) (*model.Source, error) {
	sourceUUID, err := uuid.Parse(sourceId)
	if err != nil {
		return nil, fmt.Errorf("invalid source ID %q: %w", sourceId, err)
	}
	source, err := i.store.Source().Get(ctx, sourceUUID)
	if err != nil {
		return nil, fmt.Errorf("get source %s: %w", sourceUUID, err)
	}

	return source, nil
}

func (i *ImageSvc) getSourceKey(ctx context.Context, token *jwt.Token) (interface{}, error) {
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("malformed token claims")
	}

	sourceId, ok := claims["sub"].(string)
	if !ok {
		return nil, fmt.Errorf("token missing 'sub' claim")
	}

	source, err := i.getSource(ctx, sourceId)
	if err != nil {
		return nil, fmt.Errorf("invalid source ID")
	}

	return []byte(source.ImageInfra.ImageTokenKey), nil
}

func (i *ImageSvc) generateAgentToken(ctx context.Context, source *model.Source) (string, error) {
	// get the key associated with source orgID to generate agent token
	key, err := i.store.PrivateKey().Get(ctx, source.OrgID)
	if err != nil {
		if !errors.Is(err, store.ErrRecordNotFound) {
			return "", err
		}
		newKey, token, err := auth.GenerateAgentJWTAndKey(source)
		if err != nil {
			return "", err
		}
		if _, err := i.store.PrivateKey().Create(ctx, *newKey); err != nil {
			return "", err
		}
		return token, nil
	}

	token, err := auth.GenerateAgentJWT(key, source)
	if err != nil {
		return "", err
	}

	return token, nil
}

func (i *ImageSvc) buildURL(suffix string, insecure bool, params map[string]string) (string, error) {
	base, err := url.Parse(i.baseUrl)
	if err != nil {
		return "", fmt.Errorf("failed to parse image service base URL: %v", err)
	}
	downloadURL := url.URL{
		Scheme: base.Scheme,
		Host:   base.Host,
		Path:   path.Join(base.Path, suffix),
	}
	queryValues := url.Values{}
	for k, v := range params {
		if v != "" {
			queryValues.Set(k, v)
		}
	}
	downloadURL.RawQuery = queryValues.Encode()
	if insecure {
		downloadURL.Scheme = "http"
	}
	return downloadURL.String(), nil
}

func (i *ImageSvc) downloadURL(source *model.Source, etag string) (string, time.Time, error) {
	token, exp, err := jwtForSymmetricKey([]byte(source.ImageInfra.ImageTokenKey), imageExpirationTime, source.ID.String())
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to sign image URL: %v", err)
	}

	path := fmt.Sprintf("%s/%s/%s.ova", "/api/v1/image/bytoken/", i.buildTokenETag(token, etag), source.Name)
	shortURL, err := i.buildURL(path, false, map[string]string{})
	if err != nil {
		return "", time.Time{}, err
	}

	return shortURL, exp, err
}

func jwtForSymmetricKey(key []byte, expiration time.Duration, sub string) (string, time.Time, error) {
	expTime := time.Now().Add(expiration)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp": expTime.Unix(),
		"sub": sub,
	})

	signed, err := token.SignedString(key)
	return signed, expTime, err
}
