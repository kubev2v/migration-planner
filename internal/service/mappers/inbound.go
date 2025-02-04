package mappers

import (
	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	apiAgent "github.com/kubev2v/migration-planner/api/v1alpha1/agent"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/events"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

func AgentFromSource(id uuid.UUID, user auth.User, source model.Source) model.Agent {
	return model.Agent{
		ID:         id,
		Status:     string(v1alpha1.AgentStatusNotConnected),
		StatusInfo: string(v1alpha1.AgentStatusNotConnected),
		Username:   user.Username,
		OrgID:      user.Organization,
		SourceID:   source.ID,
	}
}

func AgentFromApi(id uuid.UUID, user auth.User, resource *apiAgent.AgentStatusUpdate) model.Agent {
	return model.Agent{
		ID:         id,
		Status:     resource.Status,
		Username:   user.Username,
		OrgID:      user.Organization,
		StatusInfo: resource.StatusInfo,
		CredUrl:    resource.CredentialUrl,
		Version:    resource.Version,
		SourceID:   resource.SourceId,
	}
}

func SourceFromApi(id uuid.UUID, user auth.User, name string) model.Source {
	source := model.Source{
		ID:       id,
		Username: user.Username,
		OrgID:    user.Organization,
		Name:     name,
	}

	return source
}

func UpdateSourceFromApi(m *model.Source, inventory api.Inventory) *model.Source {
	m.Inventory = model.MakeJSONField(inventory)
	return m
}

func UIEventFromApi(apiEvent api.Event) events.UIEvent {
	uiEvent := events.UIEvent{
		CreatedAt: apiEvent.CreatedAt,
		Data:      make(map[string]string),
	}
	for _, v := range apiEvent.Data {
		uiEvent.Data[v.Key] = v.Value
	}
	return uiEvent
}
