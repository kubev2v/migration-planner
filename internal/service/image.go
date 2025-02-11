package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/image"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/pkg/metrics"
	"go.uber.org/zap"
)

func (h *ServiceHandler) GetImage(ctx context.Context, request server.GetImageRequestObject) (server.GetImageResponseObject, error) {
	writer, ok := ctx.Value(image.ResponseWriterKey).(http.ResponseWriter)
	if !ok {
		zap.S().Named("image_service").Error("failed to create ResponseWriter at GetImage")
		return server.GetImage500JSONResponse{Message: "error creating the HTTP stream"}, nil
	}

	source, err := h.store.Source().Get(ctx, request.Id)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return server.GetImage404JSONResponse{}, nil
		}
		return server.GetImage500JSONResponse{}, nil
	}

	imageBuilder := image.NewImageBuilder(source.ID)

	if source.SshPublicKey != nil {
		imageBuilder = imageBuilder.WithSshKey(*source.SshPublicKey)
	}

	if user, found := auth.UserFromContext(ctx); found {
		if user.Token != nil {
			imageBuilder = imageBuilder.WithAgentToken(user.Token.Raw)
		}
	}

	size, err := imageBuilder.Generate(ctx, writer)
	if err != nil {
		metrics.IncreaseOvaDownloadsTotalMetric("failed")
		zap.S().Named("image_service").Errorf("error generating ova at GetImage: %s", err)
		return server.GetImage500JSONResponse{Message: fmt.Sprintf("error generating image %s", err)}, nil
	}

	// Set proper headers of the OVA file:
	writer.Header().Set("Content-Type", "application/ovf")
	writer.Header().Set("Content-Length", strconv.Itoa(size))

	metrics.IncreaseOvaDownloadsTotalMetric("successful")

	return server.GetImage200ApplicationoctetStreamResponse{Body: bytes.NewReader([]byte{})}, nil
}

func (h *ServiceHandler) HeadImage(ctx context.Context, request server.HeadImageRequestObject) (server.HeadImageResponseObject, error) {
	source, err := h.store.Source().Get(ctx, request.Id)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return server.HeadImage404Response{}, nil
		}
		return server.HeadImage500Response{}, nil
	}

	imageBuilder := image.NewImageBuilder(source.ID)

	if source.SshPublicKey != nil {
		imageBuilder = imageBuilder.WithSshKey(*source.SshPublicKey)
	}

	// get token if any
	if user, found := auth.UserFromContext(ctx); found {
		if user.Token != nil {
			imageBuilder = imageBuilder.WithAgentToken(user.Token.Raw)
		}
	}

	if err := imageBuilder.Validate(); err != nil {
		zap.S().Named("image_service").Errorf("error validating at HeadImage: %s", err)
		return server.HeadImage500Response{}, nil
	}

	return server.HeadImage200Response{}, nil
}
