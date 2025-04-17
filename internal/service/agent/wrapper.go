package service

import (
	"context"

	agentServer "github.com/kubev2v/migration-planner/internal/api/server/agent"
	"go.uber.org/zap"
)

type AgentServiceHandlerLogger struct {
	delegate *AgentServiceHandler
}

func NewAgentServiceHandlerLogger(delegate *AgentServiceHandler) *AgentServiceHandlerLogger {
	return &AgentServiceHandlerLogger{delegate: delegate}
}

func (h *AgentServiceHandlerLogger) UpdateSourceInventory(ctx context.Context, request agentServer.UpdateSourceInventoryRequestObject) (agentServer.UpdateSourceInventoryResponseObject, error) {
	zap.S().Named("agent_handler").Debugw("update source inventory request",
		"source_id", request.Id,
		"agent_id", request.Body.AgentId,
		"inventory", request.Body.Inventory,
	)

	resp, err := h.delegate.UpdateSourceInventory(ctx, request)
	if err != nil {
		zap.S().Named("agent_handler").Errorw("failed to update source inventory", "error", err)
	}

	return resp, nil
}

func (h *AgentServiceHandlerLogger) UpdateAgentStatus(ctx context.Context, request agentServer.UpdateAgentStatusRequestObject) (agentServer.UpdateAgentStatusResponseObject, error) {
	zap.S().Named("agent_handler").Debugw("update agent status request",
		"agent_id", request.Id,
		"source_id", request.Body.SourceId,
		"credential_url", request.Body.CredentialUrl,
		"status", request.Body.Status,
		"version", request.Body.Version,
	)

	resp, err := h.delegate.UpdateAgentStatus(ctx, request)
	if err != nil {
		zap.S().Named("agent_handler").Errorw("failed to update agent status", "error", err)
	}

	return resp, nil
}

func (h *AgentServiceHandlerLogger) Health(ctx context.Context, request agentServer.HealthRequestObject) (agentServer.HealthResponseObject, error) {
	return h.delegate.Health(ctx, request)
}
