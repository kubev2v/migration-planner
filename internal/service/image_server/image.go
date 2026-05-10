package image_server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/version"
	"go.uber.org/zap"
)

var jwtPayloadRegexp = regexp.MustCompile(`^.+\.(.+)\..+`)

type ImageService struct {
	store store.Store
}

func NewImageService(store store.Store) *ImageService {
	return &ImageService{
		store: store,
	}
}

func (i *ImageService) ValidateToken(token string) error {
	parsedToken, err := jwt.Parse(token, i.getSourceKey)
	if err != nil {
		return fmt.Errorf("unauthorized: %v", err)
	}

	return parsedToken.Claims.Valid()
}

func (i *ImageService) getSource(ctx context.Context, sourceId string) (*model.Source, error) {
	sourceUUID, err := uuid.Parse(sourceId)
	if err != nil {
		return nil, fmt.Errorf("invalid source ID")
	}
	source, err := i.store.Source().Get(ctx, sourceUUID)
	if err != nil {
		return nil, fmt.Errorf("invalid source ID")
	}

	return source, nil
}

func (i *ImageService) UpdateAgentVersion(sourceId string) {
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

func (i *ImageService) ImageReader(ctx context.Context, sourceId string) (io.ReadSeekCloser, time.Time, error) {
	source, err := i.getSource(ctx, sourceId)
	if err != nil {
		return nil, time.Time{}, err
	}

	// Expect the agent token to be existed in this stage.
	// The Generate Url step should be responsible for saving the token
	if source.ImageInfra.AgentToken == nil || *source.ImageInfra.AgentToken == "" {
		return nil, time.Time{}, fmt.Errorf("expect agent token to be exist")
	}

	// Use pre-generated agent token from DB (stored at generate download URL creation time).
	// This ensures all pods produce byte-identical OVAs for Akamai LFO range requests.
	imageBuilder := NewImageBuilder(source.ID).WithImageInfra(source.ImageInfra).WithAgentToken(*source.ImageInfra.AgentToken)

	imageReader, _, err := imageBuilder.OpenSeekableReader(source.UpdatedAt)
	if err != nil {
		return nil, time.Time{}, err
	}

	return imageReader, source.UpdatedAt, nil
}

func (i *ImageService) IdFromJWT(jwt string) (string, error) {
	match := jwtPayloadRegexp.FindStringSubmatch(jwt)

	if len(match) != 2 {
		return "", fmt.Errorf("failed to parse JWT from URL")
	}

	decoded, err := base64.RawStdEncoding.DecodeString(match[1])
	if err != nil {
		return "", err
	}

	var p struct {
		Sub string `json:"sub"` // used by OCM tokens
	}

	err = json.Unmarshal(decoded, &p)
	if err != nil {
		return "", err
	}

	switch {
	case p.Sub != "":
		return p.Sub, nil
	}

	return "", fmt.Errorf("sub ID not found in token")
}

func (i *ImageService) getSourceKey(token *jwt.Token) (interface{}, error) {
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("malformed token claims")
	}

	sourceId, ok := claims["sub"].(string)
	if !ok {
		return nil, fmt.Errorf("token missing 'sub' claim")
	}

	source, err := i.getSource(context.TODO(), sourceId)
	if err != nil {
		return nil, fmt.Errorf("invalid source ID: %w", err)
	}

	return []byte(source.ImageInfra.ImageTokenKey), nil
}
