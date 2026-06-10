package v1alpha1

import (
	"context"
	"encoding/json"
	"fmt"

	v1alpha1 "github.com/kubev2v/migration-planner/api/v1alpha1/agent"
	agentServer "github.com/kubev2v/migration-planner/internal/api/server/agent"
	"github.com/kubev2v/migration-planner/internal/auth"
	apiMappers "github.com/kubev2v/migration-planner/internal/handlers/v1alpha1/mappers"
	"github.com/kubev2v/migration-planner/internal/handlers/validator"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
)

type AgentHandler struct {
	srv *service.AgentService
}

// Make sure we conform to servers Service interface
var _ agentServer.Service = (*AgentHandler)(nil)

func NewAgentHandler(srv *service.AgentService) *AgentHandler {
	return &AgentHandler{
		srv: srv,
	}
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
