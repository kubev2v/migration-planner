package service

import (
	"context"

	agentServer "github.com/kubev2v/migration-planner/internal/api/server/agent"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/sirupsen/logrus"
)

type AgentServiceHandler struct {
	store store.Store
	log   logrus.FieldLogger
}

// Make sure we conform to servers Service interface
var _ agentServer.Service = (*AgentServiceHandler)(nil)

func NewAgentServiceHandler(store store.Store, log logrus.FieldLogger) *AgentServiceHandler {
	return &AgentServiceHandler{
		store: store,
		log:   log,
	}
}

func (h *AgentServiceHandler) ReplaceSourceStatus(ctx context.Context, request agentServer.ReplaceSourceStatusRequestObject) (agentServer.ReplaceSourceStatusResponseObject, error) {
	result, err := h.store.Source().Update(ctx, request.Id, &request.Body.Status, &request.Body.StatusInfo, &request.Body.CredentialUrl, request.Body.Inventory)
	if err != nil {
		return agentServer.ReplaceSourceStatus400JSONResponse{}, nil
	}
	return agentServer.ReplaceSourceStatus200JSONResponse(*result), nil
}

func (h *AgentServiceHandler) Health(ctx context.Context, request agentServer.HealthRequestObject) (agentServer.HealthResponseObject, error) {
	// NO-OP
	return nil, nil
}
