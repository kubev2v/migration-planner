package service

import (
	"context"
	"strconv"

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
	id, err := strconv.ParseUint(request.Id, 10, 32)
	if err != nil {
		return agentServer.ReplaceSourceStatus400JSONResponse{Message: "invalid ID"}, nil
	}
	result, err := h.store.Source().Update(ctx, uint(id), &request.Body.Status, &request.Body.StatusInfo, &request.Body.CredentialUrl, request.Body.Inventory)
	if err != nil {
		return agentServer.ReplaceSourceStatus400JSONResponse{}, nil
	}
	return agentServer.ReplaceSourceStatus200JSONResponse(*result), nil
}
