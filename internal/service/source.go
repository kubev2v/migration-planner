package service

import (
	"context"
	"strconv"

	"github.com/kubev2v/migration-planner/internal/api/server"
)

func (h *ServiceHandler) ListSources(ctx context.Context, request server.ListSourcesRequestObject) (server.ListSourcesResponseObject, error) {
	result, err := h.store.Source().List(ctx)
	if err != nil {
		return nil, err
	}
	return server.ListSources200JSONResponse(*result), nil
}

func (h *ServiceHandler) CreateSource(ctx context.Context, request server.CreateSourceRequestObject) (server.CreateSourceResponseObject, error) {
	result, err := h.store.Source().Create(ctx, *request.Body)
	if err != nil {
		return server.CreateSource400JSONResponse{Message: err.Error()}, nil
	}
	return server.CreateSource201JSONResponse(*result), nil
}

func (h *ServiceHandler) DeleteSources(ctx context.Context, request server.DeleteSourcesRequestObject) (server.DeleteSourcesResponseObject, error) {
	err := h.store.Source().DeleteAll(ctx)
	if err != nil {
		return nil, err
	}
	return server.DeleteSources200JSONResponse{}, nil
}

func (h *ServiceHandler) ReadSource(ctx context.Context, request server.ReadSourceRequestObject) (server.ReadSourceResponseObject, error) {
	id, err := strconv.ParseUint(request.Id, 10, 32)
	if err != nil {
		return server.ReadSource400JSONResponse{Message: "invalid ID"}, nil
	}
	result, err := h.store.Source().Get(ctx, uint(id))
	if err != nil {
		return server.ReadSource404JSONResponse{}, nil
	}
	return server.ReadSource200JSONResponse(*result), nil
}

func (h *ServiceHandler) DeleteSource(ctx context.Context, request server.DeleteSourceRequestObject) (server.DeleteSourceResponseObject, error) {
	id, err := strconv.ParseUint(request.Id, 10, 32)
	if err != nil {
		return server.DeleteSource400JSONResponse{Message: "invalid ID"}, nil
	}
	err = h.store.Source().Delete(ctx, uint(id))
	if err != nil {
		return server.DeleteSource404JSONResponse{}, nil
	}
	return server.DeleteSource200JSONResponse{}, nil
}
