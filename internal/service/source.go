package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

func (h *ServiceHandler) ListSources(ctx context.Context, request server.ListSourcesRequestObject) (server.ListSourcesResponseObject, error) {
	// Get user content
	user := auth.MustHaveUser(ctx)
	filter := store.NewSourceQueryFilter().ByUsername(user.Username)

	userResult, err := h.store.Source().List(ctx, filter)
	if err != nil {
		return nil, err
	}

	includeDefault := request.Params.IncludeDefault
	if includeDefault != nil && *includeDefault {
		// Get default content
		defaultResult, err := h.store.Source().List(ctx, store.NewSourceQueryFilter().ByDefaultInventory())
		if err != nil {
			return nil, err
		}
		return server.ListSources200JSONResponse(mappers.SourceListToApi(userResult, defaultResult)), nil
	}

	return server.ListSources200JSONResponse(mappers.SourceListToApi(userResult)), nil
}

func (h *ServiceHandler) CreateSource(ctx context.Context, request server.CreateSourceRequestObject) (server.CreateSourceResponseObject, error) {
	user := auth.MustHaveUser(ctx)

	ctx, err := h.store.NewTransactionContext(ctx)
	if err != nil {
		return server.CreateSource500JSONResponse{}, nil
	}

	source := mappers.SourceFromApi(uuid.New(), user, request.Body.Name)
	result, err := h.store.Source().Create(ctx, source)
	if err != nil {
		return server.CreateSource400JSONResponse{Message: err.Error()}, nil
	}

	_, err = h.store.Agent().Create(ctx, mappers.AgentFromSource(uuid.New(), user, source))
	if err != nil {
		if _, err := store.Rollback(ctx); err != nil {
			return server.CreateSource500JSONResponse{}, nil
		}
	}

	return server.CreateSource201JSONResponse(mappers.SourceToApi(*result)), nil
}

func (h *ServiceHandler) DeleteSources(ctx context.Context, request server.DeleteSourcesRequestObject) (server.DeleteSourcesResponseObject, error) {
	ctx, err := h.store.NewTransactionContext(ctx)
	if err != nil {
		return server.DeleteSources500JSONResponse{}, nil
	}

	if err := h.store.Agent().DeleteAll(ctx); err != nil {
		return server.DeleteSources500JSONResponse{}, nil
	}

	if err := h.store.Source().DeleteAll(ctx); err != nil {
		if _, err := store.Rollback(ctx); err != nil {
			return nil, err
		}
		return server.DeleteSources500JSONResponse{}, nil
	}

	if _, err := store.Commit(ctx); err != nil {
		return server.DeleteSources500JSONResponse{}, nil
	}

	return server.DeleteSources200JSONResponse{}, nil
}

func (h *ServiceHandler) GetSource(ctx context.Context, request server.GetSourceRequestObject) (server.GetSourceResponseObject, error) {
	source, err := h.store.Source().Get(ctx, request.Id)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return server.GetSource404JSONResponse{}, nil
		}
		return server.GetSource500JSONResponse{}, nil
	}

	user := auth.MustHaveUser(ctx)
	if user.Organization != source.OrgID {
		return server.GetSource403JSONResponse{}, nil
	}

	return server.GetSource200JSONResponse(mappers.SourceToApi(*source)), nil
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

	user := auth.MustHaveUser(ctx)
	if user.Organization != source.OrgID {
		return server.DeleteSource403JSONResponse{}, nil
	}

	if err := h.store.Agent().Delete(ctx, source.Agent.ID); err != nil {
		return server.DeleteSource400JSONResponse{}, nil
	}

	if err := h.store.Source().Delete(ctx, request.Id); err != nil {
		if _, err = store.Rollback(ctx); err != nil {
			return server.DeleteSource500JSONResponse{}, nil
		}
		return server.DeleteSource404JSONResponse{}, nil
	}

	if _, err = store.Commit(ctx); err != nil {
		return server.DeleteSource500JSONResponse{}, nil
	}

	return server.DeleteSource200JSONResponse{}, nil
}

func (h *ServiceHandler) UpdateSource(ctx context.Context, request server.UpdateSourceRequestObject) (server.UpdateSourceResponseObject, error) {
	inventory := request.Body.Inventory
	id, err := uuid.Parse(inventory.Vcenter.Id)
	if err != nil {
		return server.UpdateSource500JSONResponse{}, nil
	}

	source, err := h.store.Source().Get(ctx, id)
	if err != nil {
		return server.UpdateSource500JSONResponse{}, nil
	}

	if source == nil {
		return server.UpdateSource404JSONResponse{}, nil
	}

	user := auth.MustHaveUser(ctx)
	if source.OrgID != user.Organization {
		return server.UpdateSource403JSONResponse{}, nil
	}

	source.Inventory = model.MakeJSONField(inventory)
	source.OnPremises = true
	if _, err = h.store.Source().Update(ctx, *source); err != nil {
		return server.UpdateSource500JSONResponse{}, nil
	}

	return server.UpdateSource200JSONResponse{}, nil
}
