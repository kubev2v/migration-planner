package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	agentServer "github.com/kubev2v/migration-planner/internal/api/server/agent"
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

/*
UpdateSourceInventory updates source inventory

This implements the SingleModel logic:
- Only updates for a single vCenterID are allowed
- allow two agents trying to update the source with same vCenterID
- don't allow updates from agents not belonging to the source
- don't allow updates if source is missing. (i.g the source is created as per MultiSource logic). It fails anyway because an agent always has a source.
- if the source has no inventory yet, set the vCenterID and AssociatedAgentID to this source.
*/
func (h *AgentServiceHandler) UpdateSourceInventory(ctx context.Context, request agentServer.UpdateSourceInventoryRequestObject) (agentServer.UpdateSourceInventoryResponseObject, error) {
	source, err := h.store.Source().Get(ctx, request.Id)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return agentServer.UpdateSourceInventory404JSONResponse{}, fmt.Errorf("failed to find source with id: %s", request.Id)
		}
		return agentServer.UpdateSourceInventory500JSONResponse{}, fmt.Errorf("failed to fetch source: %s", err)
	}

	agent, err := h.store.Agent().Get(ctx, request.Body.AgentId)
	if err != nil && !errors.Is(err, store.ErrRecordNotFound) {
		return agentServer.UpdateSourceInventory400JSONResponse{}, fmt.Errorf("failed to fetch the agent: %s", err)
	}

	if agent == nil {
		return agentServer.UpdateSourceInventory400JSONResponse{}, fmt.Errorf("failed to find agent %s", request.Body.AgentId)
	}

	// don't allow updates of sources not associated with this agent
	if request.Id != agent.SourceID {
		return agentServer.UpdateSourceInventory400JSONResponse{}, fmt.Errorf("request id %q does not match the agent source id %q", request.Id, agent.SourceID)
	}

	// if source has already a vCenter check if it's the same
	if source.VCenterID != "" && source.VCenterID != request.Body.Inventory.Vcenter.Id {
		return agentServer.UpdateSourceInventory400JSONResponse{}, fmt.Errorf("source's vCenter %q does not match the new inventory vCenterID %q", source.VCenterID, request.Body.Inventory.Vcenter.Id)
	}

	source = mappers.UpdateSourceFromApi(source, request.Body.Inventory)
	updatedSource, err := h.store.Source().Update(ctx, *source)
	if err != nil {
		return agentServer.UpdateSourceInventory500JSONResponse{}, fmt.Errorf("failed to update source: %s", err)
	}

	kind, inventoryEvent := h.newInventoryEvent(request.Id.String(), request.Body.Inventory)
	if err := h.eventWriter.Write(ctx, kind, inventoryEvent); err != nil {
		zap.S().Named("agent_handler").Errorw("failed to write event", "error", err, "event_kind", kind)
	}

	return agentServer.UpdateSourceInventory200JSONResponse(mappers.SourceToApi(*updatedSource)), nil
}

func (h *AgentServiceHandler) Health(ctx context.Context, request agentServer.HealthRequestObject) (agentServer.HealthResponseObject, error) {
	// NO-OP
	return nil, nil
}

// UpdateAgentStatus updates or creates a new agent resource
// If the source has not agent than the agent is created.
func (h *AgentServiceHandler) UpdateAgentStatus(ctx context.Context, request agentServer.UpdateAgentStatusRequestObject) (agentServer.UpdateAgentStatusResponseObject, error) {
	_, err := h.store.Source().Get(ctx, request.Body.SourceId)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return agentServer.UpdateAgentStatus400JSONResponse{}, fmt.Errorf("failed to find source with id: %s", request.Id)
		}
		return agentServer.UpdateAgentStatus500JSONResponse{}, fmt.Errorf("failed to fetch source: %s", err)
	}

	agent, err := h.store.Agent().Get(ctx, request.Id)
	if err != nil && !errors.Is(err, store.ErrRecordNotFound) {
		return agentServer.UpdateAgentStatus500JSONResponse{}, fmt.Errorf("failed to fetch the agent: %s", err)
	}

	if agent == nil {
		newAgent := mappers.AgentFromApi(request.Id, request.Body)
		a, err := h.store.Agent().Create(ctx, newAgent)
		if err != nil {
			return agentServer.UpdateAgentStatus400JSONResponse{}, fmt.Errorf("failed to create the agent: %s", err)
		}

		kind, agentEvent := h.newAgentEvent(mappers.AgentToApi(*a))
		if err := h.eventWriter.Write(ctx, kind, agentEvent); err != nil {
			zap.S().Named("agent_handler").Errorw("failed to write event", "error", err, "event_kind", kind)
		}

		return agentServer.UpdateAgentStatus201Response{}, nil
	}

	if _, err := h.store.Agent().Update(ctx, mappers.AgentFromApi(request.Id, request.Body)); err != nil {
		return agentServer.UpdateAgentStatus400JSONResponse{}, fmt.Errorf("failed to update agent: %s", err)
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
