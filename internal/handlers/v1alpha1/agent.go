package v1alpha1

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	v1alpha1 "github.com/kubev2v/migration-planner/api/v1alpha1/agent"
	agentServer "github.com/kubev2v/migration-planner/internal/api/server/agent"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/config"
	apiMappers "github.com/kubev2v/migration-planner/internal/handlers/v1alpha1/mappers"
	"github.com/kubev2v/migration-planner/internal/handlers/validator"
	"github.com/kubev2v/migration-planner/internal/image"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/pkg/version"
	"go.uber.org/zap"
)

type AgentHandler struct {
	srv       *service.AgentService
	sourceSrv *service.SourceService
	cfg       *config.Config
}

// Make sure we conform to servers Service interface
var _ agentServer.Service = (*AgentHandler)(nil)

func NewAgentHandler(srv *service.AgentService, sourceSrv *service.SourceService, cfg *config.Config) *AgentHandler {
	return &AgentHandler{
		srv:       srv,
		sourceSrv: sourceSrv,
		cfg:       cfg,
	}
}

// (GET /api/v1/sources/{id}/update-status)
func (h *AgentHandler) GetUpdateStatus(ctx context.Context, request agentServer.GetUpdateStatusRequestObject) (agentServer.GetUpdateStatusResponseObject, error) {
	agentJWT := auth.MustHaveAgent(ctx)
	if agentJWT.SourceID != request.Id.String() {
		return agentServer.GetUpdateStatus403JSONResponse{
			Message: fmt.Sprintf("agent is not authorized to access source %s", request.Id),
		}, nil
	}

	source, err := h.sourceSrv.GetSource(ctx, request.Id)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return agentServer.GetUpdateStatus404JSONResponse{Message: err.Error()}, nil
		default:
			zap.S().Named("agent_handler").Errorw("failed to load source", "source_id", request.Id, "error", err)
			return agentServer.GetUpdateStatus500JSONResponse{Message: fmt.Sprintf("failed to load source %s", request.Id)}, nil
		}
	}

	served := version.Get().AgentVersionName
	current := service.KnownApplianceVersion(source)
	updateAvailable := version.UpdateAvailable(served, current)

	resp := agentServer.GetUpdateStatus200JSONResponse{
		UpdateAvailable: &updateAvailable,
	}
	// An unreadable served version is not an error: no update signal, but the
	// fields stay present and empty. The same format rule as UpdateAvailable
	// decides whether latestVersion is reported at all.
	if version.IsKnownFormat(served) {
		resp.LatestVersion = &served
	}
	if current != "" {
		resp.CurrentVersion = &current
	}

	if updateAvailable {
		if version.BelowMin(current, version.MinApplianceVersion()) {
			// The appliance is too old for an in-place update; a new OVA is needed.
			supported := false
			reason := "new-ova-required"
			resp.ApplianceImage = &v1alpha1.ApplianceImage{Supported: &supported, Reason: &reason}
			return resp, nil
		}
		if !h.agentImageStaged() {
			// Nothing is served with the current ISO yet; the agent retries later.
			supported := true
			resp.ApplianceImage = &v1alpha1.ApplianceImage{Supported: &supported}
			return resp, nil
		}
		url, expireAt, err := h.sourceSrv.GetSourceDownloadURL(ctx, request.Id, image.AppliancePathName)
		if err != nil {
			zap.S().Named("agent_handler").Errorw("failed to mint agent image URL", "source_id", request.Id, "error", err)
			return agentServer.GetUpdateStatus500JSONResponse{Message: fmt.Sprintf("failed to mint agent image URL for source %s", request.Id)}, nil
		}
		supported := true
		resp.ApplianceImage = &v1alpha1.ApplianceImage{Supported: &supported, ImageUrl: &url, ExpiresAt: &expireAt}
	}

	return resp, nil
}

// agentImageStaged reports whether the appliance image tar is served with the
// current ISO; until it is staged there is nothing to point the agent at.
func (h *AgentHandler) agentImageStaged() bool {
	if h.cfg == nil {
		return false
	}
	_, err := os.Stat(image.AgentImageTarPath(h.cfg.Service.IsoPath))
	return err == nil
}

func (h *AgentHandler) UpdateSourceInventory(ctx context.Context, request agentServer.UpdateSourceInventoryRequestObject) (agentServer.UpdateSourceInventoryResponseObject, error) {
	if request.Body == nil {
		return agentServer.UpdateSourceInventory400JSONResponse{Message: "empty body"}, nil
	}

	agentJWT := auth.MustHaveAgent(ctx)
	if agentJWT.SourceID != request.Id.String() {
		return agentServer.UpdateSourceInventory403JSONResponse{
			Message: fmt.Sprintf("agent is not authorized to update source %s", request.Id),
		}, nil
	}

	if request.Body.Inventory.CreatedAt == nil {
		now := time.Now().UTC()
		request.Body.Inventory.CreatedAt = &now
	}

	data, err := json.Marshal(request.Body.Inventory)
	if err != nil {
		return agentServer.UpdateSourceInventory500JSONResponse{Message: err.Error()}, nil
	}

	updatedSource, err := h.srv.UpdateSourceInventory(ctx, mappers.InventoryUpdateForm{
		SourceID:  request.Id,
		AgentID:   request.Body.AgentId,
		Inventory: data,
		VCenterID: request.Body.Inventory.VcenterId,
	})
	if err != nil {
		switch err.(type) {
		case *service.ErrInvalidVCenterID:
			return agentServer.UpdateSourceInventory400JSONResponse{Message: err.Error()}, nil
		case *service.ErrAgentUpdateForbidden:
			return agentServer.UpdateSourceInventory403JSONResponse{Message: err.Error()}, nil
		case *service.ErrResourceNotFound:
			return agentServer.UpdateSourceInventory404JSONResponse{Message: err.Error()}, nil
		default:
			return agentServer.UpdateSourceInventory500JSONResponse{Message: err.Error()}, nil
		}
	}

	response, err := apiMappers.SourceToApi(*updatedSource)
	if err != nil {
		return agentServer.UpdateSourceInventory500JSONResponse{Message: fmt.Sprintf("failed to map source to api: %v", err)}, nil
	}

	return agentServer.UpdateSourceInventory200JSONResponse(response), nil
}

func (h *AgentHandler) UpdateSource(ctx context.Context, request agentServer.UpdateSourceRequestObject) (agentServer.UpdateSourceResponseObject, error) {
	if request.Body == nil {
		return agentServer.UpdateSource400JSONResponse{Message: "empty body"}, nil
	}

	agentJWT := auth.MustHaveAgent(ctx)
	if agentJWT.SourceID != request.Id.String() {
		return agentServer.UpdateSource403JSONResponse{
			Message: fmt.Sprintf("agent is not authorized to update source %s", request.Id),
		}, nil
	}

	// Ensure vcenter_id is consistent between inventory JSON and VCenterID field
	// If top-level VcenterId is provided, use it to override the inventory's vcenter_id
	inventory := request.Body.Inventory
	if request.Body.VcenterId != nil {
		inventory.VcenterId = *request.Body.VcenterId
	}

	if inventory.CreatedAt == nil {
		now := time.Now().UTC()
		inventory.CreatedAt = &now
	}

	data, err := json.Marshal(inventory)
	if err != nil {
		return agentServer.UpdateSource500JSONResponse{Message: err.Error()}, nil
	}

	updatedSource, err := h.srv.UpdateSource(ctx, mappers.SourceInventoryUpdateForm{
		SourceID:  request.Id,
		Inventory: data,
		VCenterID: inventory.VcenterId,
	})
	if err != nil {
		switch err.(type) {
		case *service.ErrInvalidVCenterID:
			return agentServer.UpdateSource400JSONResponse{Message: err.Error()}, nil
		case *service.ErrAgentUpdateForbidden:
			return agentServer.UpdateSource403JSONResponse{Message: err.Error()}, nil
		case *service.ErrResourceNotFound:
			return agentServer.UpdateSource404JSONResponse{Message: err.Error()}, nil
		default:
			return agentServer.UpdateSource500JSONResponse{Message: err.Error()}, nil
		}
	}

	response, err := apiMappers.SourceToApi(*updatedSource)
	if err != nil {
		return agentServer.UpdateSource500JSONResponse{Message: fmt.Sprintf("failed to map source to api: %v", err)}, nil
	}

	return agentServer.UpdateSource200JSONResponse(response), nil
}

func (h *AgentHandler) UpdateSourceSubset(ctx context.Context, request agentServer.UpdateSourceSubsetRequestObject) (agentServer.UpdateSourceSubsetResponseObject, error) {
	if request.Body == nil {
		return agentServer.UpdateSourceSubset400JSONResponse{Message: "empty body"}, nil
	}

	agentJWT := auth.MustHaveAgent(ctx)
	if agentJWT.SourceID != request.Id.String() {
		return agentServer.UpdateSourceSubset403JSONResponse{
			Message: fmt.Sprintf("agent is not authorized to update source %s", request.Id),
		}, nil
	}

	// Ensure vcenter_id is consistent between inventory JSON and VCenterID field
	// If top-level VcenterId is provided, use it to override the inventory's vcenter_id
	inventory := request.Body.Inventory
	if request.Body.VcenterId != nil {
		inventory.VcenterId = *request.Body.VcenterId
	}

	if inventory.CreatedAt == nil {
		now := time.Now().UTC()
		inventory.CreatedAt = &now
	}

	data, err := json.Marshal(inventory)
	if err != nil {
		return agentServer.UpdateSourceSubset500JSONResponse{Message: err.Error()}, nil
	}

	vmsCount := 0
	if request.Body.VmsCount != nil {
		vmsCount = *request.Body.VmsCount
	}

	subset, wasCreated, err := h.srv.UpdateSourceSubset(ctx, mappers.SourceSubsetUpdateForm{
		ID:        request.SubsetId,
		Name:      request.Body.Name,
		SourceID:  request.Id,
		VCenterID: inventory.VcenterId,
		VMsCount:  vmsCount,
		Inventory: data,
	})

	if err != nil {
		switch err.(type) {
		case *service.ErrInvalidVCenterID:
			return agentServer.UpdateSourceSubset400JSONResponse{Message: err.Error()}, nil
		case *service.ErrSourceInventoryRequired:
			return agentServer.UpdateSourceSubset400JSONResponse{Message: err.Error()}, nil
		case *service.ErrAgentUpdateForbidden:
			return agentServer.UpdateSourceSubset403JSONResponse{Message: err.Error()}, nil
		case *service.ErrResourceNotFound:
			return agentServer.UpdateSourceSubset404JSONResponse{Message: err.Error()}, nil
		default:
			return agentServer.UpdateSourceSubset500JSONResponse{Message: err.Error()}, nil
		}
	}

	response, err := apiMappers.SourceInventoryToApi(*subset)
	if err != nil {
		return agentServer.UpdateSourceSubset500JSONResponse{Message: fmt.Sprintf("failed to map source inventory to api: %v", err)}, nil
	}

	// Return 201 for creates, 200 for updates
	if wasCreated {
		return agentServer.UpdateSourceSubset201JSONResponse(response), nil
	}
	return agentServer.UpdateSourceSubset200JSONResponse(response), nil
}

func (h *AgentHandler) DeleteSourceSubset(ctx context.Context, request agentServer.DeleteSourceSubsetRequestObject) (agentServer.DeleteSourceSubsetResponseObject, error) {
	agentJWT := auth.MustHaveAgent(ctx)
	if agentJWT.SourceID != request.Id.String() {
		return agentServer.DeleteSourceSubset403JSONResponse{
			Message: fmt.Sprintf("agent is not authorized to delete subset from source %s", request.Id),
		}, nil
	}

	err := h.srv.DeleteSourceSubset(ctx, request.Id, request.SubsetId)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return agentServer.DeleteSourceSubset404JSONResponse{Message: err.Error()}, nil
		default:
			return agentServer.DeleteSourceSubset500JSONResponse{Message: err.Error()}, nil
		}
	}

	return agentServer.DeleteSourceSubset204Response{}, nil
}

// UpdateAgentStatus updates or creates a new agent resource
// If the source has not agent than the agent is created.
func (h *AgentHandler) UpdateAgentStatus(ctx context.Context, request agentServer.UpdateAgentStatusRequestObject) (agentServer.UpdateAgentStatusResponseObject, error) {
	form := v1alpha1.AgentStatusUpdate(*request.Body)

	v := validator.NewValidator()
	v.Register(validator.NewAgentValidationRules()...)
	v.RegisterStructValidation(validator.AgentStatusUpdateValidator(), v1alpha1.AgentStatusUpdate{})
	if err := v.Struct(form); err != nil {
		return agentServer.UpdateAgentStatus400JSONResponse{Message: err.Error()}, nil
	}

	agentJWT := auth.MustHaveAgent(ctx)
	if agentJWT.SourceID != request.Body.SourceId.String() {
		return agentServer.UpdateAgentStatus403JSONResponse{
			Message: fmt.Sprintf("agent is not authorized to update source %s", request.Body.SourceId),
		}, nil
	}

	_, created, err := h.srv.UpdateAgentStatus(ctx, mappers.AgentUpdateForm{
		ID:         request.Id,
		SourceID:   request.Body.SourceId,
		Status:     request.Body.Status,
		StatusInfo: request.Body.StatusInfo,
		CredUrl:    request.Body.CredentialUrl,
		Version:    request.Body.Version,
	})
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return agentServer.UpdateAgentStatus404JSONResponse{Message: err.Error()}, nil
		default:
			return agentServer.UpdateAgentStatus500JSONResponse{Message: err.Error()}, nil
		}
	}

	if created {
		return agentServer.UpdateAgentStatus201Response{}, nil
	}
	return agentServer.UpdateAgentStatus200Response{}, nil
}
