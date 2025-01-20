package service

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/events"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"go.uber.org/zap"
)

func (s *ServiceHandler) PushEvents(ctx context.Context, request server.PushEventsRequestObject) (server.PushEventsResponseObject, error) {
	uiEvent := mappers.UIEventFromApi(*request.Body)

	data, err := json.Marshal(uiEvent)
	if err != nil {
		return server.PushEvents500JSONResponse{}, nil
	}

	if err := s.eventWriter.Write(ctx, events.UIMessageKind, bytes.NewBuffer(data)); err != nil {
		zap.S().Named("service_handler").Errorw("failed to write event", "error", err, "event_kind", events.UIMessageKind)
	}

	return server.PushEvents201JSONResponse{}, nil
}
