package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

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
	imageTTL           = 30 * time.Minute
	estimatedImageSize = 3 * 1024 * 1024 * 1024 // 3 GiB
)

type fileInfo struct {
	path string
	mod  time.Time
}
type ImageSvc struct {
	store         store.Store
	tempImagesDir string
	dirLimit      string
	g             singleflight.Group
}

func NewImageSvc(s store.Store, tempImagesDir, tempImagesDirLimit string) *ImageSvc {
	return &ImageSvc{
		store:         s,
		tempImagesDir: tempImagesDir,
		dirLimit:      tempImagesDirLimit,
	}
}

func (i *ImageSvc) GenerateOVA(ctx context.Context, sourceId string) (string, string, error) {
	source, err := i.getSource(ctx, sourceId)
	if err != nil {
		return "", "", err
	}

	b := image.NewImageBuilder(source.ImageInfra.SourceID).WithImageInfra(source.ImageInfra)
	etag, err := b.Etag()
	if err != nil {
		return "", "", err
	}

	tmpfile := filepath.Join(i.tempImagesDir, fmt.Sprintf("%s.ova", etag))
	v, err, _ := i.g.Do(etag, func() (interface{}, error) {

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
		return "", "", err
	}

	return v.(string), etag, nil
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
	staleFiles, activeFiles, dirSize, err := i.listFilesSplitByAge(imageTTL)
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
