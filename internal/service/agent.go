package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/metrics"
)

const (
	defaultUpToDatePeriod = 5 * 60 * time.Second
)

type AgentService struct {
	store store.Store
}

func NewAgentService(store store.Store) *AgentService {
	return &AgentService{store: store}
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
func (as *AgentService) UpdateSourceInventory(ctx context.Context, updateForm mappers.InventoryUpdateForm) (*model.Source, error) {
	source, err := as.store.Source().Get(ctx, updateForm.SourceID)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return nil, NewErrSourceNotFound(updateForm.SourceID)
		}
		return nil, fmt.Errorf("failed to fetch source: %s", err)
	}

	agent, err := as.store.Agent().Get(ctx, updateForm.AgentID)
	if err != nil && !errors.Is(err, store.ErrRecordNotFound) {
		return nil, NewErrAgentNotFound(updateForm.AgentID)
	}

	if agent == nil {
		return nil, NewErrAgentNotFound(updateForm.AgentID)
	}

	// don't allow updates of sources not associated with this agent
	if updateForm.SourceID != agent.SourceID {
		return nil, NewErrAgentUpdateForbidden(updateForm.SourceID, updateForm.AgentID)
	}

	// if source has already a vCenter check if it's the same
	if source.VCenterID != "" && source.VCenterID != updateForm.VCenterID {
		return nil, NewErrInvalidVCenterID(updateForm.SourceID, updateForm.VCenterID)
	}

	source = mappers.UpdateSourceFromApi(source, updateForm.VCenterID, updateForm.Inventory)
	updatedSource, err := as.store.Source().Update(ctx, *source)
	if err != nil {
		return nil, fmt.Errorf("failed to update source: %s", err)
	}

	return updatedSource, nil
}

// UpdateAgentStatus updates or creates a new agent resource
// If the source has not agent than the agent is created.
func (as *AgentService) UpdateAgentStatus(ctx context.Context, updateForm mappers.AgentUpdateForm) (*model.Agent, bool, error) {
	_, err := as.store.Source().Get(ctx, updateForm.SourceID)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return nil, false, NewErrSourceNotFound(updateForm.SourceID)
		}
		return nil, false, fmt.Errorf("failed to fetch source: %s", err)
	}

	agent, err := as.store.Agent().Get(ctx, updateForm.ID)
	if err != nil && !errors.Is(err, store.ErrRecordNotFound) {
		return nil, false, fmt.Errorf("failed to fetch the agent: %s", err)
	}

	if agent == nil {
		a, err := as.store.Agent().Create(ctx, updateForm.ToModel())
		if err != nil {
			return nil, false, fmt.Errorf("failed to create the agent: %s", err)
		}

		return a, true, nil
	}

	if _, err := as.store.Agent().Update(ctx, updateForm.ToModel()); err != nil {
		return nil, false, fmt.Errorf("failed to update agent: %s", err)
	}

	// must not block here.
	// don't care about errors or context
	go as.updateMetrics()

	return agent, false, nil
}

// update metrics about agents states
// it lists all the agents and update the metrics by agent state
func (as *AgentService) updateMetrics() {
	agents, err := as.store.Agent().List(context.TODO(), store.NewAgentQueryFilter(), store.NewAgentQueryOptions())
	if err != nil {
		zap.S().Named("agent_handler").Warnw("failed to update agent metrics", "error", err)
		return
	}
	// holds the total number of agents by state
	// set defaults
	// enum: [not-connected, waiting-for-credentials, error, gathering-initial-inventory, up-to-date, source-gone]
	states := map[string]int{
		"not-connected":               0,
		"up-to-date":                  0,
		"error":                       0,
		"waiting-for-credentials":     0,
		"gathering-initial-inventory": 0,
	}
	// If agent's status has not been update for more than 5min, we consider it not-connected
	for _, a := range agents {
		status := a.Status
		if a.UpdatedAt.Before(time.Now().Add(-defaultUpToDatePeriod)) {
			status = "not-connected"
		}

		if count, ok := states[status]; ok {
			count += 1
			states[status] = count
			continue
		}

		states[status] = 1
	}
	for k, v := range states {
		metrics.UpdateAgentStateCounterMetric(k, v)
	}
}
