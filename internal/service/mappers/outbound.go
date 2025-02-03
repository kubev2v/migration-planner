package mappers

import (
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

	// We are mapping only the associated agent and ignore the rest for now.
	// Remark: If multiple agents are deployed,
	// the associated agent (i.g. the first come first serve) could be in waiting-for-creds state
	// while other agents in up-to-date states exists.
	// Which one should be presented in the API response?
	if s.AssociatedAgentID != nil {
		for _, a := range s.Agents {
			if a.ID == *s.AssociatedAgentID {
				agent := AgentToApi(a)
				source.Agent = &agent
			}
		}
	}

	if s.Inventory != nil {
		source.Inventory = &s.Inventory.Data
	}
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
