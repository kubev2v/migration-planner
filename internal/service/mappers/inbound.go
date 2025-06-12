package mappers

import (
	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	apiAgent "github.com/kubev2v/migration-planner/api/v1alpha1/agent"
	"github.com/kubev2v/migration-planner/internal/auth"
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
	}
	for k, v := range s.Labels {
		source.Labels = append(source.Labels, model.Label{Key: k, Value: v, SourceID: source.ID.String()})
	}
	return source
}

type InventoryUpdateForm struct {
	SourceID  uuid.UUID
	AgentId   uuid.UUID
	Inventory v1alpha1.Inventory
}

func AgentFromSource(id uuid.UUID, user auth.User, source model.Source) model.Agent {
	return model.Agent{
		ID:         id,
		Status:     string(v1alpha1.AgentStatusNotConnected),
		StatusInfo: string(v1alpha1.AgentStatusNotConnected),
		SourceID:   source.ID,
	}
}

func AgentFromApi(id uuid.UUID, resource *apiAgent.AgentStatusUpdate) model.Agent {
	return model.Agent{
		ID:         id,
		Status:     resource.Status,
		StatusInfo: resource.StatusInfo,
		CredUrl:    resource.CredentialUrl,
		Version:    resource.Version,
		SourceID:   resource.SourceId,
	}
}

func SourceFromApi(id uuid.UUID, user auth.User, imageTokenKey string, resource *v1alpha1.CreateSourceJSONRequestBody) model.Source {
	source := model.Source{
		ID:       id,
		Username: user.Username,
		OrgID:    user.Organization,
		Name:     resource.Name,
	}

	return source
}

func UpdateSourceFromApi(m *model.Source, inventory api.Inventory) *model.Source {
	m.Inventory = model.MakeJSONField(inventory)
	m.VCenterID = inventory.Vcenter.Id
	return m
}

func UpdateSourceOnPrem(m *model.Source, inventory api.Inventory) *model.Source {
	m.Inventory = model.MakeJSONField(inventory)
	m.VCenterID = inventory.Vcenter.Id
	m.OnPremises = true
	return m
}
