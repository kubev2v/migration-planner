package client

import (
	"context"
	"errors"
	"net"
	"sync"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1/agent"
	"github.com/kubev2v/migration-planner/internal/agent/common"
)

type Interceptor struct {
	agentStatus common.AgentStatus
	client      Planner
	l           sync.Mutex
}

func NewInterceptor(client Planner) *Interceptor {
	return &Interceptor{
		client:      client,
		agentStatus: common.AgentStatus{Connected: false},
	}
}

func (i *Interceptor) GetStatus() common.AgentStatus {
	i.l.Lock()
	defer i.l.Unlock()
	return i.agentStatus
}

func (i *Interceptor) UpdateSourceStatus(ctx context.Context, id uuid.UUID, params api.SourceStatusUpdate) error {
	i.l.Lock()
	defer i.l.Unlock()

	err := i.client.UpdateSourceStatus(ctx, id, params)
	if err != nil {
		var netOpErr *net.OpError
		if errors.As(err, &netOpErr) {
			i.agentStatus.Connected = false
			return err
		}
		i.agentStatus.Connected = true
		i.agentStatus.InventoryUpdateError = err
		i.agentStatus.InventoryUpdateSuccessfull = ptr(false)

		return err
	}

	i.agentStatus.Connected = true
	i.agentStatus.InventoryUpdateSuccessfull = ptr(true)

	return nil
}

// UpdateAgentStatus updates the agent status.
func (i *Interceptor) UpdateAgentStatus(ctx context.Context, id uuid.UUID, params api.AgentStatusUpdate) error {
	i.l.Lock()
	defer i.l.Unlock()

	err := i.client.UpdateAgentStatus(ctx, id, params)
	if err != nil {
		var netOpErr *net.OpError
		if errors.As(err, &netOpErr) {
			i.agentStatus.Connected = false
			return err
		}
		i.agentStatus.Connected = true
		i.agentStatus.StateUpdateSuccessfull = ptr(false)
		i.agentStatus.StateUpdateError = err
		return err
	}

	i.agentStatus.Connected = true
	i.agentStatus.StateUpdateSuccessfull = ptr(true)

	return nil
}

func ptr(b bool) *bool {
	return &b
}
