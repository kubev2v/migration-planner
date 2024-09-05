package service

import (
	"bytes"
	"context"
	"fmt"
	"strconv"

	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/image"
)

func (h *ServiceHandler) GetSourceImage(ctx context.Context, request server.GetSourceImageRequestObject) (server.GetSourceImageResponseObject, error) {
	id, err := strconv.ParseUint(request.Id, 10, 32)
	if err != nil {
		return server.GetSourceImage400JSONResponse{Message: "invalid ID"}, nil
	}

	i := &image.Ova{Id: id}
	_, err = i.Generate(ctx)
	if err != nil {
		return server.GetSourceImage400JSONResponse{Message: fmt.Sprintf("error generating image %s", err)}, nil
	}
	return server.GetSourceImage200ApplicationoctetStreamResponse{Body: bytes.NewReader([]byte{})}, nil
}
