package v1alpha1

import (
	"context"
	"fmt"
	"net/http"

	"github.com/kubev2v/migration-planner/pkg/middleware"

	imageServer "github.com/kubev2v/migration-planner/internal/api/server/image"
	image "github.com/kubev2v/migration-planner/internal/service/image_server"
	"github.com/kubev2v/migration-planner/pkg/metrics"
	"go.uber.org/zap"
)

type ImageHandler struct {
	is *image.ImageService
}

// Make sure we conform to servers Service interface
var _ imageServer.Service = (*ImageHandler)(nil)

func NewImageHandler(is *image.ImageService) *ImageHandler {
	return &ImageHandler{
		is: is,
	}
}

func (h *ImageHandler) Health(ctx context.Context, request imageServer.HealthRequestObject) (imageServer.HealthResponseObject, error) {
	return nil, nil
}

func (h *ImageHandler) HeadImageByToken(ctx context.Context, req imageServer.HeadImageByTokenRequestObject) (imageServer.HeadImageByTokenResponseObject, error) {
	if err := h.is.ValidateToken(req.Token); err != nil {
		return imageServer.HeadImageByToken401JSONResponse{Message: err.Error()}, nil
	}

	return imageServer.HeadImageByToken200Response{}, nil
}

func (h *ImageHandler) GetImageByToken(ctx context.Context, req imageServer.GetImageByTokenRequestObject) (imageServer.GetImageByTokenResponseObject, error) {
	if err := h.is.ValidateToken(req.Token); err != nil {
		return imageServer.GetImageByToken401JSONResponse{Message: err.Error()}, nil
	}

	sourceId, err := h.is.IdFromJWT(req.Token)
	if err != nil {
		return imageServer.GetImageByToken401JSONResponse{Message: err.Error()}, nil
	}

	writer, ok := ctx.Value(middleware.ResponseWriterKey).(http.ResponseWriter)
	httpReq, ok2 := ctx.Value(middleware.RequestKey).(*http.Request)
	if !ok || !ok2 {
		return imageServer.GetImageByToken500JSONResponse{Message: "error creating the HTTP stream"}, nil
	}

	success := false
	defer func() {
		if success {
			metrics.IncreaseOvaDownloadsTotalMetric("successful")
			return
		}
		metrics.IncreaseOvaDownloadsTotalMetric("failed")
	}()

	imageReader, modTime, err := h.is.ImageReader(ctx, sourceId)
	if err != nil {
		zap.S().Named("image_service").Errorw("failed to create seekable reader", "error", err)
		return imageServer.GetImageByToken500JSONResponse{Message: err.Error()}, nil
	}

	defer func() { _ = imageReader.Close() }()

	// Set headers before ServeContent.
	// ETag derived from source ID + creation time — deterministic across pods.
	// Must be strong (no W/ prefix) for Akamai LFO.
	etag := fmt.Sprintf(`"%s-%d"`, sourceId, modTime.Unix())
	writer.Header().Set("ETag", etag)
	writer.Header().Set("Content-Type", "application/ovf")
	writer.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, req.Name))

	// http.ServeContent handles Range requests, Content-Length, Last-Modified, etc.
	http.ServeContent(writer, httpReq, req.Name, modTime, imageReader)

	h.is.UpdateAgentVersion(sourceId)

	success = true

	// Return nil — http.ServeContent already wrote the response.
	// Returning a response object would cause a duplicate WriteHeader.
	return nil, nil
}
