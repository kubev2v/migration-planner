package model

import (
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Agent struct {
	gorm.Model
	ID         uuid.UUID `json:"id" gorm:"primaryKey"`
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

func NewAgentForSource(id uuid.UUID, source Source) Agent {
	return Agent{
		ID:       id,
		SourceID: source.ID,
	}
}
