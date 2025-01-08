package service

import (
	"context"

	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
)

func (h *ServiceHandler) ListSources(ctx context.Context, request server.ListSourcesRequestObject) (server.ListSourcesResponseObject, error) {
	filter := store.NewSourceQueryFilter()
	if user, found := auth.UserFromContext(ctx); found {
		filter = filter.ByUsername(user.Username)
	}
	result, err := h.store.Source().List(ctx, filter)
	if err != nil {
		return nil, err
	}
	return server.ListSources200JSONResponse(mappers.SourceListToApi(result)), nil
}

func (h *ServiceHandler) DeleteSources(ctx context.Context, request server.DeleteSourcesRequestObject) (server.DeleteSourcesResponseObject, error) {
	err := h.store.Source().DeleteAll(ctx)
	if err != nil {
		return nil, err
	}
	return server.DeleteSources200JSONResponse{}, nil
}

func (h *ServiceHandler) ReadSource(ctx context.Context, request server.ReadSourceRequestObject) (server.ReadSourceResponseObject, error) {
	source, err := h.store.Source().Get(ctx, request.Id)
	if err != nil {
		return server.ReadSource404JSONResponse{}, nil
	}
	if user, found := auth.UserFromContext(ctx); found {
		if user.Username != source.Username {
			return server.ReadSource403JSONResponse{}, nil
		}
	}
	return server.ReadSource200JSONResponse(mappers.SourceToApi(*source)), nil
}

func (h *ServiceHandler) DeleteSource(ctx context.Context, request server.DeleteSourceRequestObject) (server.DeleteSourceResponseObject, error) {
	// Delete the agents first
	ctx, err := h.store.NewTransactionContext(ctx)
	if err != nil {
		return server.DeleteSource404JSONResponse{}, nil
	}

	source, err := h.store.Source().Get(ctx, request.Id)
	if err != nil {
		return server.DeleteSource404JSONResponse{}, nil
	}
	if user, found := auth.UserFromContext(ctx); found {
		if user.Username != source.Username {
			return server.DeleteSource403JSONResponse{}, nil
		}
	}

	// TODO check if user is admin
	agentFilter := store.NewAgentQueryFilter().BySourceID(request.Id.String())
	if user, found := auth.UserFromContext(ctx); found {
		agentFilter = agentFilter.ByUsername(user.Username)
	}

	agents, err := h.store.Agent().List(ctx, agentFilter, store.NewAgentQueryOptions())
	if err != nil {
		return server.DeleteSource400JSONResponse{}, nil
	}

	for _, agent := range agents {
		if err := h.store.Agent().Delete(ctx, agent.ID, true); err != nil {
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
