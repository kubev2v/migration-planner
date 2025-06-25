package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/image"
	"github.com/kubev2v/migration-planner/internal/rvtools"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/internal/util"
	"go.uber.org/zap"
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

func (h *ServiceHandler) UploadRvtoolsFile(ctx context.Context, request server.UploadRvtoolsFileRequestObject) (server.UploadRvtoolsFileResponseObject, error) {
	multipartReader := request.Body
	var rvtoolsContent []byte

	source, err := h.store.Source().Get(ctx, request.Id)
	if err != nil {
		return server.UploadRvtoolsFile400JSONResponse{
			Message: "Failed to retrieve source",
		}, nil
	}

	for {
		part, err := multipartReader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return server.UploadRvtoolsFile400JSONResponse{
				Message: "Failed to read multipart data",
			}, nil
		}

		if part.FormName() == "file" {
			rvtoolsContent, err = io.ReadAll(part)
			if err != nil {
				return server.UploadRvtoolsFile400JSONResponse{
					Message: "Failed to read uploaded file content",
				}, nil
			}
			break
		}
	}

	if rvtoolsContent == nil {
		return server.UploadRvtoolsFile400JSONResponse{
			Message: "No file was found in the request",
		}, nil
	}

	if len(rvtoolsContent) == 0 {
		return server.UploadRvtoolsFile400JSONResponse{
			Message: "Empty file uploaded",
		}, nil
	}

	zap.S().Infof("Received RVTools data with size: %d bytes", len(rvtoolsContent))

	//TODO: support csv files
	if !rvtools.IsExcelFile(rvtoolsContent) {
		return server.UploadRvtoolsFile400JSONResponse{
			Message: "The uploaded file is not a valid Excel (.xlsx) file. Please upload an RVTools export in Excel format.",
		}, nil
	}

	inventory, err := rvtools.ParseRVTools(rvtoolsContent)
	if err != nil {
		return server.UploadRvtoolsFile400JSONResponse{
			Message: fmt.Sprintf("Error parsing RVTools file: %v", err),
		}, nil
	}

	if source == nil {
		return server.UploadRvtoolsFile404JSONResponse{}, nil
	}

	user := auth.MustHaveUser(ctx)
	if source.OrgID != user.Organization {
		return server.UploadRvtoolsFile403JSONResponse{}, nil
	}

	if source.VCenterID != "" && source.VCenterID != inventory.Vcenter.Id {
		return server.UploadRvtoolsFile400JSONResponse{
			Message: "vCenter ID mismatch: existing source has different vCenter ID than the uploaded RVTools file",
		}, nil
	}

	timeoutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	ctx = timeoutCtx

	var rvtoolsAgent *model.Agent
	if len(source.Agents) > 0 {
		rvtoolsAgent = &source.Agents[0]
	} else {
		newAgent := model.NewAgentForSource(uuid.New(), *source)

		if _, err := h.store.Agent().Create(ctx, newAgent); err != nil {
			_, _ = store.Rollback(ctx)
			return server.UploadRvtoolsFile500JSONResponse{}, nil
		}
		rvtoolsAgent = &newAgent
	}

	source = mappers.UpdateSourceOnPrem(source, *inventory)

	if _, err = h.store.Source().Update(ctx, *source); err != nil {
		_, _ = store.Rollback(ctx)
		return server.UploadRvtoolsFile500JSONResponse{}, nil
	}

	rvtoolsAgent.StatusInfo = "Last updated via RVTools upload on " + time.Now().Format(time.RFC3339)

	if _, err = h.store.Agent().Update(ctx, *rvtoolsAgent); err != nil {
		_, _ = store.Rollback(ctx)
		return server.UploadRvtoolsFile500JSONResponse{}, nil
	}

	if _, err := store.Commit(ctx); err != nil {
		return server.UploadRvtoolsFile500JSONResponse{}, nil
	}

	return server.UploadRvtoolsFile200JSONResponse{}, nil
}
