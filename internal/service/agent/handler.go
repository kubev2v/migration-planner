package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	agentServer "github.com/kubev2v/migration-planner/internal/api/server/agent"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/events"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/pkg/metrics"
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

	agent, err := h.store.Agent().Get(ctx, request.Body.AgentId)
	if err != nil && !errors.Is(err, store.ErrRecordNotFound) {
		return agentServer.ReplaceSourceStatus400JSONResponse{}, nil
	}

	if agent == nil {
		return agentServer.ReplaceSourceStatus400JSONResponse{}, nil
	}

	if auth.MustHaveUser(ctx).Organization != agent.OrgID {
		return agentServer.ReplaceSourceStatus403JSONResponse{}, nil
	}

	source, err := h.store.Source().Get(ctx, agent.SourceID)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return agentServer.ReplaceSourceStatus404JSONResponse{}, nil
		}
		return agentServer.ReplaceSourceStatus400JSONResponse{}, nil
	}

	source = mappers.UpdateSourceFromApi(source, request.Body.Inventory)
	result, err := h.store.Source().Update(ctx, *source)
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

	return agentServer.ReplaceSourceStatus200JSONResponse(mappers.SourceToApi(*result)), nil
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

	agent, err := h.store.Agent().Get(ctx, request.Id)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return agentServer.UpdateAgentStatus404JSONResponse{}, nil
		}
		return agentServer.UpdateAgentStatus500JSONResponse{}, nil
	}

	user := auth.MustHaveUser(ctx)
	if user.Organization != agent.OrgID {
		return agentServer.UpdateAgentStatus403JSONResponse{}, nil
	}

	if _, err := h.store.Agent().Update(ctx, mappers.AgentFromApi(request.Id, user, request.Body)); err != nil {
		_, _ = store.Rollback(ctx)
		return agentServer.UpdateAgentStatus400JSONResponse{}, nil
	}

	if _, err := store.Commit(ctx); err != nil {
		return agentServer.UpdateAgentStatus500JSONResponse{}, nil
	}

	kind, agentEvent := h.newAgentEvent(mappers.AgentToApi(*agent))
	if err := h.eventWriter.Write(ctx, kind, agentEvent); err != nil {
		zap.S().Named("agent_handler").Errorw("failed to write event", "error", err, "event_kind", kind)
	}

	// must not block here.
	// don't care about errors or context
	go h.updateMetrics()

	return agentServer.UpdateAgentStatus200Response{}, nil
}

// update metrics about agents states
// it lists all the agents and update the metrics by agent state
func (h *AgentServiceHandler) updateMetrics() {
	agents, err := h.store.Agent().List(context.TODO(), store.NewAgentQueryFilter(), store.NewAgentQueryOptions())
	if err != nil {
		zap.S().Named("agent_handler").Warnf("failed to update agent metrics: %s", err)
		return
	}
	// holds the total number of agents by state
	// set defaults
	states := map[string]int{
		string(api.AgentStatusUpToDate):                  0,
		string(api.AgentStatusError):                     0,
		string(api.AgentStatusWaitingForCredentials):     0,
		string(api.AgentStatusGatheringInitialInventory): 0,
	}
	for _, a := range agents {
		if count, ok := states[a.Status]; ok {
			count += 1
			states[a.Status] = count
			continue
		}
		states[a.Status] = 1
	}
	for k, v := range states {
		metrics.UpdateAgentStateCounterMetric(k, v)
	}
}

func (h *AgentServiceHandler) newAgentEvent(agent api.Agent) (string, io.Reader) {
	event := events.AgentEvent{
		AgentID:   agent.Id.String(),
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
