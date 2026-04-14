package v1alpha1

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	imageServer "github.com/kubev2v/migration-planner/internal/api/server/image"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/image"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/metrics"
	"github.com/kubev2v/migration-planner/pkg/version"
	"go.uber.org/zap"
)

type ImageHandler struct {
	store store.Store
	cfg   *config.Config
}

// Make sure we conform to servers Service interface
var _ imageServer.Service = (*ImageHandler)(nil)

func NewImageHandler(store store.Store, cfg *config.Config) *ImageHandler {
	return &ImageHandler{
		store: store,
		cfg:   cfg,
	}
}

func (h *ImageHandler) Health(ctx context.Context, request imageServer.HealthRequestObject) (imageServer.HealthResponseObject, error) {
	return nil, nil
}

func (h *ImageHandler) HeadImageByToken(ctx context.Context, req imageServer.HeadImageByTokenRequestObject) (imageServer.HeadImageByTokenResponseObject, error) {
	if err := image.ValidateToken(ctx, req.Token, h.getSourceKey); err != nil {
		return imageServer.HeadImageByToken401JSONResponse{Message: err.Error()}, nil
	}

	sourceId, err := image.IdFromJWT(req.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to create the HTTP stream: %v", err)
	}
	_, err = h.getSource(ctx, sourceId)
	if err != nil {
		return imageServer.HeadImageByToken401JSONResponse{Message: "failed to create the HTTP stream"}, nil
	}

	return imageServer.HeadImageByToken200Response{}, nil
}

func (h *ImageHandler) GetImageByToken(ctx context.Context, req imageServer.GetImageByTokenRequestObject) (imageServer.GetImageByTokenResponseObject, error) {
	writer, ok := ctx.Value(image.ResponseWriterKey).(http.ResponseWriter)
	if !ok {
		return imageServer.GetImageByToken500JSONResponse{Message: "error creating the HTTP stream"}, nil
	}

	if err := image.ValidateToken(ctx, req.Token, h.getSourceKey); err != nil {
		return imageServer.GetImageByToken401JSONResponse{Message: err.Error()}, nil
	}

	sourceId, err := image.IdFromJWT(req.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to create the HTTP stream: %v", err)
	}
	source, err := h.getSource(ctx, sourceId)
	if err != nil {
		return imageServer.GetImageByToken401JSONResponse{Message: "failed to create the HTTP stream"}, nil
	}

	imageBuilder := image.NewImageBuilder(source.ID)
	imageBuilder.WithImageInfra(source.ImageInfra)

	if err := generateAndSetAgentToken(ctx, source, h.store, imageBuilder); err != nil {
		return imageServer.GetImageByToken500JSONResponse{}, nil
	}

	// Get image size
	size, err := imageBuilder.Size()
	if err != nil {
		return imageServer.GetImageByToken500JSONResponse{Message: fmt.Sprintf("failed to read image size %s", err)}, nil
	}

	// Set proper headers of the OVA file:
	writer.Header().Set("Content-Type", "application/ovf")
	writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", req.Name))
	writer.Header().Set("Content-Length", strconv.FormatUint(size, 10))

	err = imageBuilder.Generate(ctx, writer)
	if err != nil {
		metrics.IncreaseOvaDownloadsTotalMetric("failed")
		zap.S().Named("image_service").Errorw("failed to generate ova at GetImage", "error", err)
		return imageServer.GetImageByToken500JSONResponse{Message: fmt.Sprintf("failed to generate image %s", err)}, nil
	}

	metrics.IncreaseOvaDownloadsTotalMetric("successful")

	versionInfo := version.Get()
	if !version.IsValidAgentVersion(versionInfo.AgentVersionName) {
		zap.S().Named("image_service").Warnw("agent version not valid, skipping storage", "source_id", source.ID, "agent_version_name", versionInfo.AgentVersionName, "agent_git_commit", versionInfo.AgentGitCommit)
	} else {
		// Use detached context to ensure version persists even if client disconnects
		persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Use atomic update to prevent race conditions during concurrent downloads
		if err := h.store.ImageInfra().UpdateAgentVersion(persistCtx, source.ID.String(), versionInfo.AgentVersionName); err != nil {
			zap.S().Named("image_service").Warnw("failed to update agent version", "error", err, "source_id", source.ID, "agent_version", versionInfo.AgentVersionName)
		} else {
			zap.S().Named("image_service").Infow("stored agent version", "source_id", source.ID, "agent_version", versionInfo.AgentVersionName)
		}
	}

	return imageServer.GetImageByToken200ApplicationovfResponse{Body: bytes.NewReader([]byte{})}, nil
}

func generateAndSetAgentToken(ctx context.Context, source *model.Source, storeInstance store.Store, imageBuilder *image.ImageBuilder) error {
	// get the key associated with source orgID to generate agent token
	key, err := storeInstance.PrivateKey().Get(ctx, source.OrgID)
	if err != nil {
		if !errors.Is(err, store.ErrRecordNotFound) {
			return err
		}
		newKey, token, err := auth.GenerateAgentJWTAndKey(source)
		if err != nil {
			return err
		}
		if _, err := storeInstance.PrivateKey().Create(ctx, *newKey); err != nil {
			return err
		}
		imageBuilder.WithAgentToken(token)
	} else {
		token, err := auth.GenerateAgentJWT(key, source)
		if err != nil {
			return err
		}
		imageBuilder.WithAgentToken(token)
	}
	return nil
}

func (h *ImageHandler) getSourceKey(token *jwt.Token) (interface{}, error) {
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("malformed token claims")
	}

	sourceId, ok := claims["sub"].(string)
	if !ok {
		return nil, fmt.Errorf("token missing 'sub' claim")
	}

	source, err := h.getSource(context.TODO(), sourceId)
	if err != nil {
		return imageServer.GetImageByToken500JSONResponse{Message: "invalid source ID"}, nil
	}

	return []byte(source.ImageInfra.ImageTokenKey), nil
}

func (h *ImageHandler) getSource(ctx context.Context, sourceId string) (*model.Source, error) {
	sourceUUID, err := uuid.Parse(sourceId)
	if err != nil {
		return nil, fmt.Errorf("invalid source ID")
	}
	source, err := h.store.Source().Get(ctx, sourceUUID)
	if err != nil {
		return nil, fmt.Errorf("invalid source ID")
	}

	return source, nil
}
