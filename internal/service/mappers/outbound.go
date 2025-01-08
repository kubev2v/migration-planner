package mappers

import (
	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

func SourceToApi(s model.Source) api.Source {
	source := api.Source{
		Id:        s.ID,
		Inventory: nil,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}

	if len(s.Agents) > 0 {
		agents := make([]api.SourceAgentItem, 0, len(s.Agents))
		for _, a := range s.Agents {
			agents = append(agents, api.SourceAgentItem{Id: uuid.MustParse(a.ID), Associated: a.Associated})
		}
		source.Agents = &agents
	}

	if s.Inventory != nil {
		source.Inventory = &s.Inventory.Data
	}
	return source
}

func SourceListToApi(sources model.SourceList) api.SourceList {
	sourceList := make([]api.Source, 0, len(sources))
	for _, source := range sources {
		sourceList = append(sourceList, SourceToApi(source))
	}
	return sourceList
}

func AgentToApi(a model.Agent) api.Agent {
	agent := api.Agent{
		Id:            a.ID,
		Status:        api.StringToAgentStatus(a.Status),
		StatusInfo:    a.StatusInfo,
		CreatedAt:     a.CreatedAt,
		UpdatedAt:     a.UpdatedAt,
		CredentialUrl: a.CredUrl,
		Version:       a.Version,
		Associated:    a.Associated,
	}

	if a.DeletedAt.Valid {
		agent.DeletedAt = &a.DeletedAt.Time
	}

	if a.SourceID != nil {
		agent.SourceId = a.SourceID
	}

	return agent
}

func AgentListToApi(agents model.AgentList) api.AgentList {
	agentList := make([]api.Agent, 0, len(agents))
	for _, agent := range agents {
		agentList = append(agentList, AgentToApi(agent))
	}
	return agentList
}
