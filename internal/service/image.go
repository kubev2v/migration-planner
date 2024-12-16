package service

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/image"
)

func (h *ServiceHandler) GetImage(ctx context.Context, request server.GetImageRequestObject) (server.GetImageResponseObject, error) {
	writer, ok := ctx.Value(image.ResponseWriterKey).(http.ResponseWriter)
	if !ok {
		return server.GetImage500JSONResponse{Message: "error creating the HTTP stream"}, nil
	}

	ova := &image.Ova{SshKey: request.Params.SshKey, Writer: writer}
	// get token if any
	if user, found := auth.UserFromContext(ctx); found {
		ova.Jwt = user.Token
	}
	if err := ova.Generate(); err != nil {
		return server.GetImage500JSONResponse{Message: fmt.Sprintf("error generating image %s", err)}, nil
	}
	return server.GetImage200ApplicationoctetStreamResponse{Body: bytes.NewReader([]byte{})}, nil
}

func (h *ServiceHandler) HeadImage(ctx context.Context, request server.HeadImageRequestObject) (server.HeadImageResponseObject, error) {
	writer, ok := ctx.Value(image.ResponseWriterKey).(http.ResponseWriter)
	if !ok {
		return server.HeadImage500Response{}, nil
	}
	ova := &image.Ova{SshKey: request.Params.SshKey, Writer: writer}
	if err := ova.Validate(); err != nil {
		return server.HeadImage500Response{}, nil
	}
	return server.HeadImage200Response{}, nil
}
