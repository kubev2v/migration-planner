package mappers

import (
	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	apiAgent "github.com/kubev2v/migration-planner/api/v1alpha1/agent"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

func AgentFromApi(username string, orgID string, resource *apiAgent.AgentStatusUpdate) model.Agent {
	return model.Agent{
		ID:         resource.Id,
		Status:     resource.Status,
		StatusInfo: resource.StatusInfo,
		Username:   username,
		OrgID:      orgID,
		CredUrl:    resource.CredentialUrl,
		Version:    resource.Version,
	}
}

func SourceFromApi(id uuid.UUID, username string, orgID string, inventory *api.Inventory, onPremises bool) model.Source {
	source := model.Source{
		ID:         id,
		Username:   username,
		OrgID:      orgID,
		OnPremises: onPremises,
	}

	if inventory != nil {
		source.Inventory = model.MakeJSONField(*inventory)
	}

	return source
}
