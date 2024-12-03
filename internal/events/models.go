package events

import (
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
