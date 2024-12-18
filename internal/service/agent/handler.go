package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	agentServer "github.com/kubev2v/migration-planner/internal/api/server/agent"
	"github.com/kubev2v/migration-planner/internal/events"
	"github.com/kubev2v/migration-planner/internal/store"
	"go.uber.org/zap"
)

type AgentServiceHandler struct {
	store       store.Store
	eventWriter *events.EventProducer
}

// Make sure we conform to servers Service interface
var _ agentServer.Service = (*AgentServiceHandler)(nil)

func NewAgentServiceHandler(store store.Store, ew *events.EventProducer) *AgentServiceHandler {
	return &AgentServiceHandler{
		store:       store,
		eventWriter: ew,
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

	kind, inventoryEvent := h.newInventoryEvent(request.Id.String(), request.Body.Inventory)
	if err := h.eventWriter.Write(ctx, kind, inventoryEvent); err != nil {
		zap.S().Named("agent_handler").Errorw("failed to write event", "error", err, "event_kind", kind)
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
		a, err := h.store.Agent().Create(ctx, *request.Body)
		if err != nil {
			return agentServer.UpdateAgentStatus400JSONResponse{}, nil
		}
		if _, err := store.Commit(ctx); err != nil {
			return agentServer.UpdateAgentStatus500JSONResponse{}, nil
		}

		kind, agentEvent := h.newAgentEvent(*a)
		if err := h.eventWriter.Write(ctx, kind, agentEvent); err != nil {
			zap.S().Named("agent_handler").Errorw("failed to write event", "error", err, "event_kind", kind)
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

	kind, agentEvent := h.newAgentEvent(*agent)
	if err := h.eventWriter.Write(ctx, kind, agentEvent); err != nil {
		zap.S().Named("agent_handler").Errorw("failed to write event", "error", err, "event_kind", kind)
	}

	return agentServer.UpdateAgentStatus200Response{}, nil
}

func (h *AgentServiceHandler) newAgentEvent(agent api.Agent) (string, io.Reader) {
	event := events.AgentEvent{
		AgentID:   agent.Id,
		State:     string(agent.Status),
		StateInfo: agent.StatusInfo,
	}

	data, _ := json.Marshal(event)

	return events.AgentMessageKind, bytes.NewReader(data)
}

func (h *AgentServiceHandler) newInventoryEvent(sourceID string, inventory api.Inventory) (string, io.Reader) {
	event := events.InventoryEvent{
		SourceID:  sourceID,
		Inventory: inventory,
	}

	data, _ := json.Marshal(event)

	return events.InventoryMessageKind, bytes.NewReader(data)
}
