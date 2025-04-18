package service

import (
	"context"
	"errors"

	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/image"
	"github.com/kubev2v/migration-planner/internal/store"
	"go.uber.org/zap"
)

func (h *ServiceHandler) HeadImage(ctx context.Context, request server.HeadImageRequestObject) (server.HeadImageResponseObject, error) {
	source, err := h.store.Source().Get(ctx, request.Id)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return server.HeadImage404Response{}, nil
		}
		return server.HeadImage500Response{}, nil
	}

	imageBuilder := image.NewImageBuilder(source.ID)

	if source.ImageInfra.SshPublicKey != "" {
		imageBuilder = imageBuilder.WithSshKey(source.ImageInfra.SshPublicKey)
	}

	// get token if any
	if user, found := auth.UserFromContext(ctx); found {
		if user.Token != nil {
			imageBuilder = imageBuilder.WithAgentToken(user.Token.Raw)
		}
	}

	if err := imageBuilder.Validate(); err != nil {
		zap.S().Named("image_service").Errorw("error validating at HeadImage", "error", err)
		return server.HeadImage500Response{}, nil
	}

	return server.HeadImage200Response{}, nil
}
