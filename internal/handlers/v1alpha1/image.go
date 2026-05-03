package v1alpha1

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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
	httpReq, ok := ctx.Value(image.RequestKey).(*http.Request)
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

	// Use pre-generated agent token from DB (stored at download URL creation time).
	// This ensures all pods produce byte-identical OVAs for Akamai LFO range requests.
	if source.ImageInfra.AgentToken != nil && *source.ImageInfra.AgentToken != "" {
		imageBuilder.WithAgentToken(*source.ImageInfra.AgentToken)
	} else {
		// Fallback for pre-migration sources: generate on the fly
		if err := generateAndSetAgentToken(ctx, source, h.store, imageBuilder); err != nil {
			return imageServer.GetImageByToken500JSONResponse{}, nil
		}
	}

	// Use source.CreatedAt as deterministic ModTime for TAR headers
	modTime := source.CreatedAt

	reader, _, err := imageBuilder.OpenSeekableReader(modTime)
	if err != nil {
		metrics.IncreaseOvaDownloadsTotalMetric("failed")
		zap.S().Named("image_service").Errorw("failed to create seekable reader", "error", err)
		return imageServer.GetImageByToken500JSONResponse{Message: fmt.Sprintf("failed to create seekable reader: %s", err)}, nil
	}
	defer func() { _ = reader.Close() }()

	// Set headers before ServeContent
	writer.Header().Set("Content-Type", "application/ovf")
	writer.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, req.Name))

	// http.ServeContent handles Range requests, Content-Length, Last-Modified, etc.
	http.ServeContent(writer, httpReq, req.Name, modTime, reader)

	metrics.IncreaseOvaDownloadsTotalMetric("successful")

	versionInfo := version.Get()
	if !version.IsValidAgentVersion(versionInfo.AgentVersionName) {
		zap.S().Named("image_service").Warnw("agent version not valid, skipping storage", "source_id", source.ID, "agent_version_name", versionInfo.AgentVersionName, "agent_git_commit", versionInfo.AgentGitCommit)
	} else {
		persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := h.store.ImageInfra().UpdateAgentVersion(persistCtx, source.ID.String(), versionInfo.AgentVersionName); err != nil {
			zap.S().Named("image_service").Warnw("failed to update agent version", "error", err, "source_id", source.ID, "agent_version", versionInfo.AgentVersionName)
		} else {
			zap.S().Named("image_service").Infow("stored agent version", "source_id", source.ID, "agent_version", versionInfo.AgentVersionName)
		}
	}

	// Return nil — http.ServeContent already wrote the response.
	// Returning a response object would cause a duplicate WriteHeader.
	return nil, nil
}

func generateAndSetAgentToken(ctx context.Context, source *model.Source, storeInstance store.Store, imageBuilder *image.ImageBuilder) error {
	// get the key associated with source orgID to generate agent token
	var token string
	key, err := storeInstance.PrivateKey().Get(ctx, source.OrgID)
	if err != nil {
		if !errors.Is(err, store.ErrRecordNotFound) {
			return err
		}
		newKey, t, err := auth.GenerateAgentJWTAndKey(source)
		if err != nil {
			return err
		}
		if _, err := storeInstance.PrivateKey().Create(ctx, *newKey); err != nil {
			return err
		}
		token = t
	} else {
		t, err := auth.GenerateAgentJWT(key, source)
		if err != nil {
			return err
		}
		token = t
	}

	imageBuilder.WithAgentToken(token)

	// Persist so subsequent requests produce byte-identical OVAs (required for Akamai LFO)
	if err := storeInstance.ImageInfra().UpdateAgentToken(ctx, source.ID.String(), token); err != nil {
		zap.S().Named("image_service").Warnw("failed to persist fallback agent token", "error", err, "source_id", source.ID)
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
		return nil, fmt.Errorf("invalid source ID: %w", err)
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
