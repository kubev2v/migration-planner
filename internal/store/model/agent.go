package model

import (
	"encoding/json"

	"gorm.io/gorm"
)

type Agent struct {
	gorm.Model
	ID         string `json:"id" gorm:"primaryKey"`
	Username   string
	OrgID      string
	Status     string
	StatusInfo string
	CredUrl    string
	SourceID   *string
	Source     *Source
	Version    string
	Associated bool
}

type AgentList []Agent

func (a Agent) String() string {
	v, _ := json.Marshal(a)
	return string(v)
}

func NewAgentFromID(id string) *Agent {
	return &Agent{ID: id}
}
