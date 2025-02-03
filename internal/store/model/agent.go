package model

import (
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Agent struct {
	gorm.Model
	ID         uuid.UUID `json:"id" gorm:"primaryKey"`
	Username   string
	OrgID      string
	Status     string
	StatusInfo string
	CredUrl    string
	Version    string
	SourceID   uuid.UUID
}

type AgentList []Agent

func (a Agent) String() string {
	v, _ := json.Marshal(a)
	return string(v)
}

func NewAgentFromID(id uuid.UUID) *Agent {
	return &Agent{ID: id}
}
