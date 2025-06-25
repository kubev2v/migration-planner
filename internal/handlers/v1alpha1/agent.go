package v1alpha1

import (
	"context"

	v1alpha1 "github.com/kubev2v/migration-planner/api/v1alpha1/agent"
	agentServer "github.com/kubev2v/migration-planner/internal/api/server/agent"
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

	updatedSource, err := h.srv.UpdateSourceInventory(ctx, mappers.InventoryUpdateForm{
		SourceID:  request.Id,
		AgentId:   request.Body.AgentId,
		Inventory: request.Body.Inventory,
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

	return agentServer.UpdateSourceInventory200JSONResponse(apiMappers.SourceToApi(*updatedSource)), nil
}

// UpdateAgentStatus updates or creates a new agent resource
// If the source has not agent than the agent is created.
func (h *AgentHandler) UpdateAgentStatus(ctx context.Context, request agentServer.UpdateAgentStatusRequestObject) (agentServer.UpdateAgentStatusResponseObject, error) {
	form := v1alpha1.AgentStatusUpdate(*request.Body)

	v := validator.NewValidator()
	v.Register(validator.NewAgentValidationRules()...)
	if err := v.Struct(form); err != nil {
		return agentServer.UpdateAgentStatus400JSONResponse{Message: err.Error()}, nil
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
