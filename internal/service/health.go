package service

import (
	"context"

	"github.com/kubev2v/migration-planner/internal/api/server"
)

func (h *ServiceHandler) Health(ctx context.Context, request server.HealthRequestObject) (server.HealthResponseObject, error) {
	return server.Health200Response{}, nil
}
