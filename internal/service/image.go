package service

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/image"
	"github.com/kubev2v/migration-planner/pkg/metrics"
	"go.uber.org/zap"
)

func (h *ServiceHandler) GetImage(ctx context.Context, request server.GetImageRequestObject) (server.GetImageResponseObject, error) {
	writer, ok := ctx.Value(image.ResponseWriterKey).(http.ResponseWriter)
	if !ok {
		zap.S().Named("image_service").Error("failed to create ResponseWriter at GetImage")
		return server.GetImage500JSONResponse{Message: "error creating the HTTP stream"}, nil
	}
	ova := &image.Ova{SshKey: request.Params.SshKey, Writer: writer}

	// get token if any
	if user, found := auth.UserFromContext(ctx); found {
		ova.Jwt = user.Token
	}

	// Calculate the size of the OVA, so the download show estimated time:
	size, err := ova.OvaSize()
	if err != nil {
		zap.S().Named("image_service").Errorf("error calculating OvaSize at GetImage: %s", err)
		return server.GetImage500JSONResponse{Message: "error creating the HTTP stream"}, nil
	}

	// Set proper headers of the OVA file:
	writer.Header().Set("Content-Type", "application/ovf")
	writer.Header().Set("Content-Length", strconv.Itoa(size))

	// Generate the OVA image
	if err := ova.Generate(); err != nil {
		metrics.IncreaseOvaDownloadsTotalMetric("failed")
		zap.S().Named("image_service").Errorf("error generating ova at GetImage: %s", err)
		return server.GetImage500JSONResponse{Message: fmt.Sprintf("error generating image %s", err)}, nil
	}

	metrics.IncreaseOvaDownloadsTotalMetric("successful")

	return server.GetImage200ApplicationoctetStreamResponse{Body: bytes.NewReader([]byte{})}, nil
}

func (h *ServiceHandler) HeadImage(ctx context.Context, request server.HeadImageRequestObject) (server.HeadImageResponseObject, error) {
	writer, ok := ctx.Value(image.ResponseWriterKey).(http.ResponseWriter)
	if !ok {
		return server.HeadImage500Response{}, nil
	}
	ova := &image.Ova{SshKey: request.Params.SshKey, Writer: writer}
	if err := ova.Validate(); err != nil {
		zap.S().Named("image_service").Errorf("error validating at HeadImage: %s", err)
		return server.HeadImage500Response{}, nil
	}
	return server.HeadImage200Response{}, nil
}
