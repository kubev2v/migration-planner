package mappers

import (
	"slices"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

func SourceToApi(s model.Source) api.Source {
	source := api.Source{
		Id:         s.ID,
		Inventory:  nil,
		CreatedAt:  s.CreatedAt,
		UpdatedAt:  s.UpdatedAt,
		OnPremises: s.OnPremises,
		Name:       s.Name,
	}

	if s.Inventory != nil {
		source.Inventory = &s.Inventory.Data
	}

	if len(s.Labels) > 0 {
		labels := make([]api.Label, 0, len(s.Labels))
		for _, label := range s.Labels {
			labels = append(labels, api.Label{Key: label.Key, Value: label.Value})
		}
		source.Labels = &labels
	}

	// We are mapping only the first agent based on created_at timestamp and ignore the rest for now.
	// TODO:
	// Remark: If multiple agents are deployed, we pass only the first one based on created_at timestamp
	// while other agents in up-to-date states exists.
	// Which one should be presented in the API response?
	if len(s.Agents) == 0 {
		return source
	}

	slices.SortFunc(s.Agents, func(a model.Agent, b model.Agent) int {
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		if a.CreatedAt.After(b.CreatedAt) {
			return 1
		}
		return 0
	})
	agent := AgentToApi(s.Agents[0])
	source.Agent = &agent

	return source
}

func SourceListToApi(sources ...model.SourceList) api.SourceList {
	sourceList := []api.Source{}
	for _, source := range sources {
		for _, s := range source {
			sourceList = append(sourceList, SourceToApi(s))
		}
	}
	return sourceList
}

func AgentToApi(a model.Agent) api.Agent {
	return api.Agent{
		Id:            a.ID,
		Status:        api.StringToAgentStatus(a.Status),
		StatusInfo:    a.StatusInfo,
		CreatedAt:     a.CreatedAt,
		UpdatedAt:     a.UpdatedAt,
		CredentialUrl: a.CredUrl,
		Version:       a.Version,
	}
}
