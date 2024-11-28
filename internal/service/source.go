package service

import (
	"context"

	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/store"
)

func (h *ServiceHandler) ListSources(ctx context.Context, request server.ListSourcesRequestObject) (server.ListSourcesResponseObject, error) {
	result, err := h.store.Source().List(ctx)
	if err != nil {
		return nil, err
	}
	return server.ListSources200JSONResponse(result), nil
}

func (h *ServiceHandler) DeleteSources(ctx context.Context, request server.DeleteSourcesRequestObject) (server.DeleteSourcesResponseObject, error) {
	err := h.store.Source().DeleteAll(ctx)
	if err != nil {
		return nil, err
	}
	return server.DeleteSources200JSONResponse{}, nil
}

func (h *ServiceHandler) ReadSource(ctx context.Context, request server.ReadSourceRequestObject) (server.ReadSourceResponseObject, error) {
	result, err := h.store.Source().Get(ctx, request.Id)
	if err != nil {
		return server.ReadSource404JSONResponse{}, nil
	}
	return server.ReadSource200JSONResponse(*result), nil
}

func (h *ServiceHandler) DeleteSource(ctx context.Context, request server.DeleteSourceRequestObject) (server.DeleteSourceResponseObject, error) {
	// Delete the agents first
	ctx, err := h.store.NewTransactionContext(ctx)
	if err != nil {
		return server.DeleteSource404JSONResponse{}, nil
	}

	agents, err := h.store.Agent().List(ctx, store.NewAgentQueryFilter().BySourceID(request.Id.String()), store.NewAgentQueryOptions())
	if err != nil {
		return server.DeleteSource400JSONResponse{}, nil
	}

	for _, agent := range agents {
		if err := h.store.Agent().Delete(ctx, agent.Id, true); err != nil {
			_, _ = store.Rollback(ctx)
			return server.DeleteSource400JSONResponse{}, nil
		}
	}

	if err := h.store.Source().Delete(ctx, request.Id); err != nil {
		_, _ = store.Rollback(ctx)
		return server.DeleteSource404JSONResponse{}, nil
	}

	_, _ = store.Commit(ctx)
	return server.DeleteSource200JSONResponse{}, nil
}
