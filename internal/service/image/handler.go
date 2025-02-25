package service

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	imageServer "github.com/kubev2v/migration-planner/internal/api/server/image"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/events"
	"github.com/kubev2v/migration-planner/internal/image"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/metrics"
	"go.uber.org/zap"
)

type ImageHandler struct {
	store       store.Store
	eventWriter *events.EventProducer
	cfg         *config.Config
}

// Make sure we conform to servers Service interface
var _ imageServer.Service = (*ImageHandler)(nil)

func NewImageHandler(store store.Store, ew *events.EventProducer, cfg *config.Config) *ImageHandler {
	return &ImageHandler{
		store:       store,
		eventWriter: ew,
		cfg:         cfg,
	}
}

func (h *ImageHandler) Health(ctx context.Context, request imageServer.HealthRequestObject) (imageServer.HealthResponseObject, error) {
	return nil, nil
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
		return nil, fmt.Errorf("error creating the HTTP stream: %v", err)
	}
	source, err := h.getSource(ctx, sourceId)
	if err != nil {
		return imageServer.GetImageByToken401JSONResponse{Message: "error creating the HTTP stream"}, nil
	}

	imageBuilder := image.NewImageBuilder(source.ID)
	if source.ImageInfra.SshPublicKey != "" {
		imageBuilder = imageBuilder.WithSshKey(source.ImageInfra.SshPublicKey)
	}

	// TODO: We need to fetch the pull-secret from source
	size, err := imageBuilder.Generate(ctx, writer)
	if err != nil {
		metrics.IncreaseOvaDownloadsTotalMetric("failed")
		zap.S().Named("image_service").Errorf("error generating ova at GetImage: %s", err)
		return imageServer.GetImageByToken500JSONResponse{Message: fmt.Sprintf("error generating image %s", err)}, nil
	}

	// Set proper headers of the OVA file:
	writer.Header().Set("Content-Type", "application/ovf")
	writer.Header().Set("Content-Length", strconv.Itoa(size))
	writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", req.Name))

	metrics.IncreaseOvaDownloadsTotalMetric("successful")

	return imageServer.GetImageByToken200ApplicationoctetStreamResponse{Body: bytes.NewReader([]byte{})}, nil
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
