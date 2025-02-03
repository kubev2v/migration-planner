package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	agentServer "github.com/kubev2v/migration-planner/internal/api/server/agent"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/events"
	"github.com/kubev2v/migration-planner/internal/image"
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

func (h *AgentServiceHandler) GetImageByToken(ctx context.Context, req agentServer.GetImageByTokenRequestObject) (agentServer.GetImageByTokenResponseObject, error) {
	writer, ok := ctx.Value(image.ResponseWriterKey).(http.ResponseWriter)
	if !ok {
		return agentServer.GetImageByToken500JSONResponse{Message: "error creating the HTTP stream"}, nil
	}

	// TODO: parse token

	sourceId, err := image.IdFromJWT(req.Token)
	if err != nil {
		return agentServer.GetImageByToken500JSONResponse{Message: "error creating the HTTP stream"}, nil
	}
	sourceUUID, err := uuid.Parse(sourceId)
	if err != nil {
		return agentServer.GetImageByToken500JSONResponse{Message: "invalid source ID"}, nil
	}
	source, err := h.store.Source().Get(ctx, sourceUUID)
	if err != nil {
		return agentServer.GetImageByToken500JSONResponse{Message: "invalid source ID"}, nil
	}

	ova := &image.Ova{SourceID: source.ID, SshKey: source.SshPublicKey, Writer: writer}

	// Calculate the size of the OVA, so the download show estimated time:
	size, err := ova.OvaSize()
	if err != nil {
		return agentServer.GetImageByToken500JSONResponse{Message: "error creating the HTTP stream"}, nil
	}

	// Set proper headers of the OVA file:
	writer.Header().Set("Content-Type", "application/ovf")
	writer.Header().Set("Content-Length", strconv.Itoa(size))
	writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename='%s'", req.Imagename))

	// Generate the OVA image
	if err := ova.Generate(); err != nil {
		metrics.IncreaseOvaDownloadsTotalMetric("failed")
		return agentServer.GetImageByToken500JSONResponse{Message: fmt.Sprintf("error generating image %s", err)}, nil
	}

	metrics.IncreaseOvaDownloadsTotalMetric("successful")

	return agentServer.GetImageByToken200ApplicationoctetStreamResponse{Body: bytes.NewReader([]byte{})}, nil
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
	// start new transaction
	ctx, err := h.store.NewTransactionContext(ctx)
	if err != nil {
		return agentServer.UpdateSourceInventory500JSONResponse{}, nil
	}

	source, err := h.store.Source().Get(ctx, request.Id)
	if err != nil {
		if errors.Is(store.ErrRecordNotFound, err) {
			return agentServer.UpdateSourceInventory404JSONResponse{}, nil
		}
		return agentServer.UpdateSourceInventory500JSONResponse{}, nil
	}

	agent, err := h.store.Agent().Get(ctx, request.Body.AgentId)
	if err != nil && !errors.Is(err, store.ErrRecordNotFound) {
		return agentServer.UpdateSourceInventory400JSONResponse{}, nil
	}

	if agent == nil {
		return agentServer.UpdateSourceInventory400JSONResponse{}, nil
	}

	// agent must be under organization's scope
	if auth.MustHaveUser(ctx).Organization != agent.OrgID {
		return agentServer.UpdateSourceInventory403JSONResponse{}, nil
	}

	// don't allow updates of sources not associated with this agent
	if request.Id != agent.SourceID {
		return agentServer.UpdateSourceInventory400JSONResponse{Message: "sss"}, nil
	}

	// if source has already a vCenter check if it's the same
	if source.VCenterID != nil && *source.VCenterID != request.Body.Inventory.Vcenter.Id {
		return agentServer.UpdateSourceInventory400JSONResponse{}, nil
	}

	source = mappers.UpdateSourceFromApi(source, request.Body.Inventory)
	// if this is the first time updating the source
	// associate this agent with source and set vCenterID
	if source.AssociatedAgentID == nil {
		source.AssociatedAgentID = &request.Body.AgentId
	}

	if source.VCenterID == nil {
		source.VCenterID = &request.Body.Inventory.Vcenter.Id
	}

	updatedSource, err := h.store.Source().Update(ctx, *source)
	if err != nil {
		_, _ = store.Rollback(ctx)
		return agentServer.UpdateSourceInventory400JSONResponse{}, nil
	}

	if _, err := store.Commit(ctx); err != nil {
		return agentServer.UpdateSourceInventory500JSONResponse{}, nil
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
	ctx, err := h.store.NewTransactionContext(ctx)
	if err != nil {
		return agentServer.UpdateAgentStatus500JSONResponse{}, nil
	}

	user := auth.MustHaveUser(ctx)

	source, err := h.store.Source().Get(ctx, request.Body.SourceId)
	if err != nil {
		if errors.Is(store.ErrRecordNotFound, err) {
			return agentServer.UpdateAgentStatus400JSONResponse{}, nil
		}
		return agentServer.UpdateAgentStatus500JSONResponse{}, nil
	}

	agent, err := h.store.Agent().Get(ctx, request.Id)
	if err != nil && !errors.Is(err, store.ErrRecordNotFound) {
		return agentServer.UpdateAgentStatus500JSONResponse{}, nil
	}

	if agent == nil {
		// try to find the source and check if
		newAgent := mappers.AgentFromApi(request.Id, user, request.Body)
		a, err := h.store.Agent().Create(ctx, newAgent)
		if err != nil {
			return agentServer.UpdateAgentStatus400JSONResponse{}, nil
		}

		// if this is the first created agent registered with the source id
		id := uuid.MustParse(request.Id.String())
		source.AssociatedAgentID = &id
		if _, err := h.store.Source().Update(ctx, *source); err != nil {
			_, _ = store.Rollback(ctx)
			return agentServer.UpdateAgentStatus500JSONResponse{}, nil
		}

		if _, err := store.Commit(ctx); err != nil {
			return agentServer.UpdateAgentStatus500JSONResponse{}, nil
		}

		kind, agentEvent := h.newAgentEvent(mappers.AgentToApi(*a))
		if err := h.eventWriter.Write(ctx, kind, agentEvent); err != nil {
			zap.S().Named("agent_handler").Errorw("failed to write event", "error", err, "event_kind", kind)
		}

		return agentServer.UpdateAgentStatus201Response{}, nil
	}

	if user.Organization != agent.OrgID {
		return agentServer.UpdateAgentStatus403JSONResponse{}, nil
	}

	if _, err := h.store.Agent().Update(ctx, mappers.AgentFromApi(request.Id, user, request.Body)); err != nil {
		_, _ = store.Rollback(ctx)
		return agentServer.UpdateAgentStatus400JSONResponse{}, nil
	}

	kind, agentEvent := h.newAgentEvent(mappers.AgentToApi(*agent))
	if err := h.eventWriter.Write(ctx, kind, agentEvent); err != nil {
		zap.S().Named("agent_handler").Errorw("failed to write event", "error", err, "event_kind", kind)
	}

	if _, err := store.Commit(ctx); err != nil {
		return agentServer.UpdateAgentStatus500JSONResponse{}, nil
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
