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

	ctx, err := h.store.NewTransactionContext(ctx)
	if err != nil {
		return server.CreateSource500JSONResponse{}, nil
	}

	// Generate a signing key for tokens for the source
	// TODO: merge imageTokenKey and sshPublickKey with the rest of image infra
	imageTokenKey, err := image.HMACKey(32)
	if err != nil {
		return server.CreateSource400JSONResponse{Message: err.Error()}, nil
	}

	source := mappers.SourceFromApi(uuid.New(), user, imageTokenKey, request.Body)
	result, err := h.store.Source().Create(ctx, source)
	if err != nil {
		return server.CreateSource400JSONResponse{Message: err.Error()}, nil
	}

	imageInfra := mappers.ImageInfraFromApi(source.ID, imageTokenKey, request.Body)
	if _, err := h.store.ImageInfra().Create(ctx, imageInfra); err != nil {
		_, _ = store.Rollback(ctx)
		return server.CreateSource500JSONResponse{}, nil
	}

	if _, err := store.Commit(ctx); err != nil {
		return server.CreateSource500JSONResponse{}, nil
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

func (h *ServiceHandler) UpdateSourceInventory(ctx context.Context, request server.UpdateSourceInventoryRequestObject) (server.UpdateSourceInventoryResponseObject, error) {
	ctx, err := h.store.NewTransactionContext(ctx)
	if err != nil {
		return server.UpdateSourceInventory404JSONResponse{}, nil
	}

	source, err := h.store.Source().Get(ctx, request.Id)
	if err != nil {
		return server.UpdateSourceInventory400JSONResponse{}, nil
	}

	if source == nil {
		return server.UpdateSourceInventory404JSONResponse{}, nil
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
			return server.UpdateSourceInventory500JSONResponse{}, nil
		}
	}

	user := auth.MustHaveUser(ctx)
	if source.OrgID != user.Organization {
		return server.UpdateSourceInventory403JSONResponse{}, nil
	}

	if source.VCenterID != "" && source.VCenterID != request.Body.Inventory.Vcenter.Id {
		_, _ = store.Rollback(ctx)
		return server.UpdateSourceInventory400JSONResponse{}, nil
	}

	source = mappers.UpdateSourceFromApi(source, request.Body.Inventory)

	if _, err = h.store.Source().Update(ctx, *source); err != nil {
		_, _ = store.Rollback(ctx)
		return server.UpdateSourceInventory500JSONResponse{}, nil
	}

	if _, err := store.Commit(ctx); err != nil {
		return server.UpdateSourceInventory500JSONResponse{}, nil
	}

	return server.UpdateSourceInventory200JSONResponse(mappers.SourceToApi(*source)), nil
}

func (h *ServiceHandler) UpdateSourceMetadata(ctx context.Context, request server.UpdateSourceMetadataRequestObject) (server.UpdateSourceMetadataResponseObject, error) {
	ctx, err := h.store.NewTransactionContext(ctx)
	if err != nil {
		return server.UpdateSourceMetadata500JSONResponse{Message: "Failed to start transaction: " + err.Error()}, nil
	}

	source, err := h.store.Source().Get(ctx, request.Id) // Get preloads ImageInfra
	if err != nil {
		_, _ = store.Rollback(ctx)
		if errors.Is(err, store.ErrRecordNotFound) {
			return server.UpdateSourceMetadata404JSONResponse{Message: "Source not found"}, nil
		}
		return server.UpdateSourceMetadata500JSONResponse{Message: "Failed to get source: " + err.Error()}, nil
	}

	user := auth.MustHaveUser(ctx)
	if source.OrgID != user.Organization {
		_, _ = store.Rollback(ctx)
		return server.UpdateSourceMetadata403JSONResponse{Message: "Forbidden"}, nil
	}

	updated := false

	if request.Body.Name != nil && *request.Body.Name != source.Name {
		source.Name = *request.Body.Name
		updated = true
	}

	if request.Body.Labels != nil {
		source.Labels = mappers.LabelsFromApi(request.Body.Labels)
		updated = true
	}

	// ImageInfra is part of the Source model and should be preloaded by Get.
	// GORM's association handling should persist changes to source.ImageInfra when source is updated.
	if request.Body.SshPublicKey != nil && *request.Body.SshPublicKey != source.ImageInfra.SshPublicKey {
		source.ImageInfra.SshPublicKey = *request.Body.SshPublicKey
		updated = true
	}
	if request.Body.CertificateChain != nil && *request.Body.CertificateChain != source.ImageInfra.CertificateChain {
		source.ImageInfra.CertificateChain = *request.Body.CertificateChain
		updated = true
	}

	if request.Body.Proxy != nil {
		if request.Body.Proxy.HttpUrl != nil && *request.Body.Proxy.HttpUrl != source.ImageInfra.HttpProxyUrl {
			source.ImageInfra.HttpProxyUrl = *request.Body.Proxy.HttpUrl
			updated = true
		}
		if request.Body.Proxy.HttpsUrl != nil && *request.Body.Proxy.HttpsUrl != source.ImageInfra.HttpsProxyUrl {
			source.ImageInfra.HttpsProxyUrl = *request.Body.Proxy.HttpsUrl
			updated = true
		}
		if request.Body.Proxy.NoProxy != nil && *request.Body.Proxy.NoProxy != source.ImageInfra.NoProxyDomains {
			source.ImageInfra.NoProxyDomains = *request.Body.Proxy.NoProxy
			updated = true
		}
	}

	if updated {
		// The ImageInfra model is associated with Source. GORM should handle updates to
		// source.ImageInfra when source is updated if ImageInfra's primary key (SourceID)
		// is correctly set up and ImageInfra was loaded with the Source.
		// store.ImageInfra() only has Create, implying updates go via the Source model.
		updatedSource, updateErr := h.store.Source().Update(ctx, *source)
		if updateErr != nil {
			_, _ = store.Rollback(ctx)
			return server.UpdateSourceMetadata500JSONResponse{Message: "Failed to update source metadata: " + updateErr.Error()}, nil
		}
		source = updatedSource // Use the returned updated source
	}

	if _, err := store.Commit(ctx); err != nil {
		return server.UpdateSourceMetadata500JSONResponse{Message: "Failed to commit transaction: " + err.Error()}, nil
	}

	return server.UpdateSourceMetadata200JSONResponse(mappers.SourceToApi(*source)), nil
}
