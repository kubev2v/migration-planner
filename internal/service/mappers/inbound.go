package mappers

import (
	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

type SourceCreateForm struct {
	CertificateChain string
	Name             string
	HttpUrl          string
	HttpsUrl         string
	NoProxy          string
	SshPublicKey     string
	Username         string
	OrgID            string
	Organization     string
	UserLastName     string
	UserFirstName    string
	Labels           map[string]string
}

func (s SourceCreateForm) ToImageInfra(sourceID uuid.UUID, imageTokenKey string) model.ImageInfra {
	imageInfra := model.ImageInfra{
		SourceID:         sourceID,
		ImageTokenKey:    imageTokenKey,
		SshPublicKey:     s.SshPublicKey,
		CertificateChain: s.CertificateChain,
		HttpProxyUrl:     s.HttpUrl,
		HttpsProxyUrl:    s.HttpsUrl,
		NoProxyDomains:   s.NoProxy,
	}
	return imageInfra
}

func (s SourceCreateForm) ToSource() model.Source {
	source := model.Source{
		ID:       uuid.New(),
		Username: s.Username,
		OrgID:    s.OrgID,
		Name:     s.Name,
		Labels:   make([]model.Label, 0),
	}
	for k, v := range s.Labels {
		source.Labels = append(source.Labels, model.Label{Key: k, Value: v, SourceID: source.ID.String()})
	}

	user := model.User{
		Username:     s.Username,
		Organization: s.Organization,
		LastName:     s.UserLastName,
		FirstName:    s.UserFirstName,
	}

	source.User = &user

	return source
}

type InventoryUpdateForm struct {
	SourceID  uuid.UUID
	AgentId   uuid.UUID
	Inventory v1alpha1.Inventory // TODO: think about versioning. This is bound to v1alpha1 currently.
}

type AgentUpdateForm struct {
	ID         uuid.UUID
	Status     string
	StatusInfo string
	CredUrl    string
	Version    string
	SourceID   uuid.UUID
}

func (f *AgentUpdateForm) ToModel() model.Agent {
	return model.Agent{
		ID:         f.ID,
		Status:     f.Status,
		StatusInfo: f.StatusInfo,
		CredUrl:    f.CredUrl,
		Version:    f.Version,
		SourceID:   f.SourceID,
	}
}

func UpdateSourceFromApi(m *model.Source, inventory api.Inventory) *model.Source {
	m.Inventory = model.MakeJSONField(inventory)
	m.VCenterID = inventory.Vcenter.Id
	return m
}
