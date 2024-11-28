package agent

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	agentapi "github.com/kubev2v/migration-planner/api/v1alpha1/agent"
	"github.com/kubev2v/migration-planner/internal/agent/client"
	"github.com/kubev2v/migration-planner/pkg/log"
)

type InventoryUpdater struct {
	log        *log.PrefixLogger
	client     client.Planner
	agentID    uuid.UUID
	prevStatus []byte
}

type InventoryData struct {
	Inventory api.Inventory `json:"inventory"`
	Error     string        `json:"error"`
}

func NewInventoryUpdater(log *log.PrefixLogger, agentID uuid.UUID, client client.Planner) *InventoryUpdater {
	updater := &InventoryUpdater{
		log:        log,
		client:     client,
		agentID:    agentID,
		prevStatus: []byte{},
	}
	return updater
}

func (u *InventoryUpdater) UpdateServiceWithInventory(ctx context.Context, inventory *api.Inventory) {
	update := agentapi.SourceStatusUpdate{
		Inventory: *inventory,
		AgentId:   u.agentID,
	}

	newContents, err := json.Marshal(update)
	if err != nil {
		u.log.Errorf("failed marshalling new status: %v", err)
	}
	if bytes.Equal(u.prevStatus, newContents) {
		u.log.Debug("Local status did not change, skipping service update")
		return
	}

	err = u.client.UpdateSourceStatus(ctx, uuid.MustParse(inventory.Vcenter.Id), update)
	if err != nil {
		u.log.Errorf("failed updating status: %v", err)
		return
	}

	u.prevStatus = newContents
}
