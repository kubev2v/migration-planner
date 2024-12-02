package service

import (
	"context"
	"errors"

	agentServer "github.com/kubev2v/migration-planner/internal/api/server/agent"
	"github.com/kubev2v/migration-planner/internal/store"
	"go.uber.org/zap"
)

type AgentServiceHandler struct {
	store store.Store
}

// Make sure we conform to servers Service interface
var _ agentServer.Service = (*AgentServiceHandler)(nil)

func NewAgentServiceHandler(store store.Store) *AgentServiceHandler {
	return &AgentServiceHandler{
		store: store,
	}
}

func (h *AgentServiceHandler) ReplaceSourceStatus(ctx context.Context, request agentServer.ReplaceSourceStatusRequestObject) (agentServer.ReplaceSourceStatusResponseObject, error) {
	// start new transaction
	ctx, err := h.store.NewTransactionContext(ctx)
	if err != nil {
		return agentServer.ReplaceSourceStatus500JSONResponse{}, nil
	}

	agent, err := h.store.Agent().Get(ctx, request.Body.AgentId.String())
	if err != nil && !errors.Is(err, store.ErrRecordNotFound) {
		return agentServer.ReplaceSourceStatus400JSONResponse{}, nil
	}

	if agent == nil {
		return agentServer.ReplaceSourceStatus400JSONResponse{}, nil
	}

	source, err := h.store.Source().Get(ctx, request.Id)
	if err != nil && !errors.Is(err, store.ErrRecordNotFound) {
		return agentServer.ReplaceSourceStatus400JSONResponse{}, nil
	}

	associated := false
	if source == nil {
		source, err = h.store.Source().Create(ctx, request.Id)
		if err != nil {
			return agentServer.ReplaceSourceStatus400JSONResponse{}, nil
		}
		associated = true
	}

	// connect the agent to the source
	// If agent is already connected to a source but the source is different from the current one, connect it anyway.
	// An agent is allowed to change sources.
	if agent.SourceId == nil || *agent.SourceId != source.Id.String() {
		if agent, err = h.store.Agent().UpdateSourceID(ctx, agent.Id, request.Id.String(), associated); err != nil {
			_, _ = store.Rollback(ctx)
			return agentServer.ReplaceSourceStatus400JSONResponse{}, nil
		}
	}

	// We are not allowing updates from agents not associated with the source ("first come first serve").
	if !agent.Associated {
		zap.S().Errorf("Failed to update status of source %s from agent %s. Agent is not the associated with the source", source.Id, agent.Id)
		if _, err := store.Commit(ctx); err != nil {
			return agentServer.ReplaceSourceStatus500JSONResponse{}, nil
		}
		return agentServer.ReplaceSourceStatus400JSONResponse{}, nil
	}

	result, err := h.store.Source().Update(ctx, request.Id, &request.Body.Inventory)
	if err != nil {
		_, _ = store.Rollback(ctx)
		return agentServer.ReplaceSourceStatus400JSONResponse{}, nil
	}

	if _, err := store.Commit(ctx); err != nil {
		return agentServer.ReplaceSourceStatus500JSONResponse{}, nil
	}

	return agentServer.ReplaceSourceStatus200JSONResponse(*result), nil
}

func (h *AgentServiceHandler) Health(ctx context.Context, request agentServer.HealthRequestObject) (agentServer.HealthResponseObject, error) {
	// NO-OP
	return nil, nil
}

func (h *AgentServiceHandler) UpdateAgentStatus(ctx context.Context, request agentServer.UpdateAgentStatusRequestObject) (agentServer.UpdateAgentStatusResponseObject, error) {
	ctx, err := h.store.NewTransactionContext(ctx)
	if err != nil {
		return agentServer.UpdateAgentStatus500JSONResponse{}, nil
	}
	agent, err := h.store.Agent().Get(ctx, request.Id.String())
	if err != nil && !errors.Is(err, store.ErrRecordNotFound) {
		return agentServer.UpdateAgentStatus400JSONResponse{}, nil
	}

	if agent == nil {
		if _, err := h.store.Agent().Create(ctx, *request.Body); err != nil {
			return agentServer.UpdateAgentStatus400JSONResponse{}, nil
		}
		if _, err := store.Commit(ctx); err != nil {
			return agentServer.UpdateAgentStatus500JSONResponse{}, nil
		}
		return agentServer.UpdateAgentStatus201Response{}, nil
	}

	// check if agent is marked for deletion
	if agent.DeletedAt != nil {
		return agentServer.UpdateAgentStatus410JSONResponse{}, nil
	}

	if _, err := h.store.Agent().Update(ctx, *request.Body); err != nil {
		_, _ = store.Rollback(ctx)
		return agentServer.UpdateAgentStatus400JSONResponse{}, nil
	}

	if _, err := store.Commit(ctx); err != nil {
		return agentServer.UpdateAgentStatus500JSONResponse{}, nil
	}
	return agentServer.UpdateAgentStatus200Response{}, nil
}
