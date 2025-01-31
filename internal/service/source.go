package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/image"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/internal/util"
)

func (h *ServiceHandler) GetSourceDownloadURL(ctx context.Context, request server.GetSourceDownloadURLRequestObject) (server.GetSourceDownloadURLResponseObject, error) {
	source, err := h.store.Source().Get(ctx, request.Id)
	if err != nil {
		return server.GetSourceDownloadURL404JSONResponse{}, nil
	}

	// FIXME: refactor the environment vars + config.yaml
	baseUrl := util.GetEnv("MIGRATION_PLANNER_IMAGE_URL", "http://localhost:11443")
	newURL, expiresAt, err := image.GenerateDownloadURLByToken(baseUrl, source)
	if err != nil {
		return server.GetSourceDownloadURL400JSONResponse{}, err
	}

	return server.GetSourceDownloadURL200JSONResponse{Url: newURL, ExpiresAt: (*time.Time)(expiresAt)}, nil
}

func (h *ServiceHandler) ListSources(ctx context.Context, request server.ListSourcesRequestObject) (server.ListSourcesResponseObject, error) {
	// Get user content
	user := auth.MustHaveUser(ctx)
	filter := store.NewSourceQueryFilter().ByOrgID(user.Organization)

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

	// Generate a signing key for tokens for the source
	imageTokenKey, err := image.HMACKey(32)
	if err != nil {
		return server.CreateSource400JSONResponse{Message: err.Error()}, nil
	}

	source := mappers.SourceFromApi(uuid.New(), user, imageTokenKey, request.Body)
	result, err := h.store.Source().Create(ctx, source)
	if err != nil {
		return server.CreateSource400JSONResponse{Message: err.Error()}, nil
	}

	return server.CreateSource201JSONResponse(mappers.SourceToApi(*result)), nil
}

func (h *ServiceHandler) DeleteSources(ctx context.Context, request server.DeleteSourcesRequestObject) (server.DeleteSourcesResponseObject, error) {
	if err := h.store.Source().DeleteAll(ctx); err != nil {
		if _, err := store.Rollback(ctx); err != nil {
			return nil, err
		}
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
	source, err := h.store.Source().Get(ctx, request.Id)
	if err != nil {
		return server.DeleteSource404JSONResponse{}, nil
	}

	user := auth.MustHaveUser(ctx)
	if user.Organization != source.OrgID {
		return server.DeleteSource403JSONResponse{}, nil
	}

	if err := h.store.Source().Delete(ctx, request.Id); err != nil {
		return server.DeleteSource500JSONResponse{}, nil
	}

	return server.DeleteSource200JSONResponse{}, nil
}

func (h *ServiceHandler) UpdateSource(ctx context.Context, request server.UpdateSourceRequestObject) (server.UpdateSourceResponseObject, error) {
	ctx, err := h.store.NewTransactionContext(ctx)
	if err != nil {
		return server.UpdateSource404JSONResponse{}, nil
	}

	source, err := h.store.Source().Get(ctx, request.Id)
	if err != nil {
		return server.UpdateSource400JSONResponse{}, nil
	}

	if source == nil {
		return server.UpdateSource404JSONResponse{}, nil
	}

	// create the agent if missing
	var agent *model.Agent
	for _, a := range source.Agents {
		if a.ID == request.Body.AgentId {
			agent = &a
			break
		}
	}

	if agent == nil {
		newAgent := model.NewAgentForSource(uuid.New(), *source)
		if _, err := h.store.Agent().Create(ctx, newAgent); err != nil {
			return server.UpdateSource500JSONResponse{}, nil
		}
	}

	user := auth.MustHaveUser(ctx)
	if source.OrgID != user.Organization {
		return server.UpdateSource403JSONResponse{}, nil
	}

	if source.VCenterID != "" && source.VCenterID != request.Body.Inventory.Vcenter.Id {
		_, _ = store.Rollback(ctx)
		return server.UpdateSource400JSONResponse{}, nil
	}

	source = mappers.UpdateSourceOnPrem(source, request.Body.Inventory)

	if _, err = h.store.Source().Update(ctx, *source); err != nil {
		_, _ = store.Rollback(ctx)
		return server.UpdateSource500JSONResponse{}, nil
	}

	if _, err := store.Commit(ctx); err != nil {
		return server.UpdateSource500JSONResponse{}, nil
	}

	return server.UpdateSource200JSONResponse{}, nil
}
