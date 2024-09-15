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
		return server.GetSourceImage400JSONResponse{Message: "error creating the HTTP stream"}, nil
	}
	ova := &image.Ova{Id: request.Id, Writer: writer}
	if err := ova.Generate(); err != nil {
		return server.GetSourceImage400JSONResponse{Message: fmt.Sprintf("error generating image %s", err)}, nil
	}
	return server.GetSourceImage200ApplicationoctetStreamResponse{Body: bytes.NewReader([]byte{})}, nil
}
