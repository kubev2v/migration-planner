package model

import (
	"encoding/json"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	apiAgent "github.com/kubev2v/migration-planner/api/v1alpha1/agent"
	"gorm.io/gorm"
)

type Agent struct {
	gorm.Model
	ID         string `json:"id" gorm:"primaryKey"`
	Status     string
	StatusInfo string
	CredUrl    string
	SourceID   *string
	Source     *Source
	Version    string
}

type AgentList []Agent

func (a Agent) String() string {
	v, _ := json.Marshal(a)
	return string(v)
}

func NewAgentFromID(id string) *Agent {
	return &Agent{ID: id}
}

func NewAgentFromApiResource(resource *apiAgent.AgentStatusUpdate) *Agent {
	return &Agent{
		ID:         resource.Id,
		Status:     resource.Status,
		StatusInfo: resource.StatusInfo,
		CredUrl:    resource.CredentialUrl,
		Version:    resource.Version,
	}
}

func (a *Agent) ToApiResource() api.Agent {
	agent := api.Agent{
		Id:            a.ID,
		Status:        api.StringToAgentStatus(a.Status),
		StatusInfo:    a.StatusInfo,
		CreatedAt:     a.CreatedAt,
		UpdatedAt:     a.UpdatedAt,
		CredentialUrl: a.CredUrl,
		Version:       a.Version,
	}

	if a.SourceID != nil {
		agent.SourceId = a.SourceID
	}

	return agent
}

func (al AgentList) ToApiResource() api.AgentList {
	agentList := make([]api.Agent, len(al))
	for i, agent := range al {
		agentList[i] = agent.ToApiResource()
	}
	return agentList
}
