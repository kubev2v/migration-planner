package service

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/image"
)

func (h *ServiceHandler) GetSourceImage(ctx context.Context, request server.GetSourceImageRequestObject) (server.GetSourceImageResponseObject, error) {
	writer, ok := ctx.Value(image.ResponseWriterKey).(http.ResponseWriter)
	if !ok {
		return server.GetSourceImage500JSONResponse{Message: "error creating the HTTP stream"}, nil
	}
	result, err := h.store.Source().Get(ctx, request.Id)
	if err != nil {
		return server.GetSourceImage404JSONResponse{}, nil
	}
	ova := &image.Ova{Id: request.Id, SshKey: result.SshKey, Writer: writer}
	if err := ova.Generate(); err != nil {
		return server.GetSourceImage500JSONResponse{Message: fmt.Sprintf("error generating image %s", err)}, nil
	}
	return server.GetSourceImage200ApplicationoctetStreamResponse{Body: bytes.NewReader([]byte{})}, nil
}

func (h *ServiceHandler) HeadSourceImage(ctx context.Context, request server.HeadSourceImageRequestObject) (server.HeadSourceImageResponseObject, error) {
	writer, ok := ctx.Value(image.ResponseWriterKey).(http.ResponseWriter)
	if !ok {
		return server.HeadSourceImage500Response{}, nil
	}
	result, err := h.store.Source().Get(ctx, request.Id)
	if err != nil {
		return server.HeadSourceImage404Response{}, nil
	}
	ova := &image.Ova{Id: request.Id, SshKey: result.SshKey, Writer: writer}
	if err := ova.Validate(); err != nil {
		return server.HeadSourceImage500Response{}, nil
	}
	return server.HeadSourceImage200Response{}, nil
}
