package events

import (
	"time"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
)

type InventoryEvent struct {
	SourceID  string        `json:"source_id"`
	Inventory api.Inventory `json:"inventory"`
}

type AgentEvent struct {
	AgentID   string `json:"agent_id"`
	State     string `json:"state"`
	StateInfo string `json:"state_info"`
}

type UIEvent struct {
	CreatedAt time.Time         `json:"created_at"`
	Data      map[string]string `json:"data"`
}
