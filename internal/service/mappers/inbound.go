package mappers

import (
	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

type SourceCreateForm struct {
	CertificateChain string
	Name             string
	SshPublicKey     string
	Username         string
	OrgID            string
	Labels           *[]api.Label
	Proxy            *api.AgentProxy
}

// applyProxySettings applies proxy settings from an API proxy to an ImageInfra model
func applyProxySettings(proxy *api.AgentProxy, imageInfra *model.ImageInfra) {
	if proxy != nil {
		if proxy.HttpUrl != nil {
			imageInfra.HttpProxyUrl = *proxy.HttpUrl
		}
		if proxy.HttpsUrl != nil {
			imageInfra.HttpsProxyUrl = *proxy.HttpsUrl
		}
		if proxy.NoProxy != nil {
			imageInfra.NoProxyDomains = *proxy.NoProxy
		}
	}
}

func (s SourceCreateForm) ToImageInfra(sourceID uuid.UUID, imageTokenKey string) model.ImageInfra {
	imageInfra := model.ImageInfra{
		SourceID:         sourceID,
		ImageTokenKey:    imageTokenKey,
		SshPublicKey:     s.SshPublicKey,
		CertificateChain: s.CertificateChain,
	}

	// Handle proxy settings consistently
	applyProxySettings(s.Proxy, &imageInfra)

	return imageInfra
}

func (s SourceCreateForm) ToSource() model.Source {
	sourceID := uuid.New()
	source := model.Source{
		ID:       sourceID,
		Username: s.Username,
		OrgID:    s.OrgID,
		Name:     s.Name,
		Labels:   LabelsFromApi(sourceID, s.Labels),
	}
	return source
}

type InventoryUpdateForm struct {
	SourceID  uuid.UUID
	AgentId   uuid.UUID
	Inventory api.Inventory // TODO: think about versioning. This is bound to v1alpha1 currently.
}

type AgentUpdateForm struct {
	ID         uuid.UUID
	Status     string
	StatusInfo string
	CredUrl    string
	Version    string
	SourceID   uuid.UUID
}

type SourceUpdateForm struct {
	Name             *string
	Labels           *[]api.Label
	SshPublicKey     *string
	CertificateChain *string
	Proxy            *api.AgentProxy
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

func LabelsFromApi(sourceID uuid.UUID, apiLabels *[]api.Label) []model.Label {
	if apiLabels == nil {
		return nil
	}

	// Return empty slice if there are no labels
	if len(*apiLabels) == 0 {
		return []model.Label{}
	}

	modelLabels := make([]model.Label, len(*apiLabels))
	for i, apiLabel := range *apiLabels {
		modelLabels[i] = model.Label{
			Key:      apiLabel.Key,
			Value:    apiLabel.Value,
			SourceID: sourceID.String(),
		}
	}
	return modelLabels
}

func AgentProxyFromApi(apiProxy *api.AgentProxy) *model.JSONField[api.AgentProxy] {
	if apiProxy == nil {
		return nil
	}
	return model.MakeJSONField(*apiProxy)
}

func UpdateSourceFromApi(m *model.Source, inventory api.Inventory) *model.Source {
	m.Inventory = model.MakeJSONField(inventory)
	m.VCenterID = inventory.Vcenter.Id
	return m
}

// Apply applies the update form to an existing Source and returns the updated ImageInfra copy together with the calculated labels slice (if labels were provided).
func (f SourceUpdateForm) Apply(src *model.Source) (model.ImageInfra, []model.Label) {
	// Work with a local copy of the ImageInfra to avoid mutating the original before validation/persistence
	img := src.ImageInfra

	// Basic scalar fields on Source
	if f.Name != nil {
		src.Name = *f.Name
	}

	// Labels – convert from API and attach to source if provided
	var labels []model.Label
	if f.Labels != nil {
		labels = LabelsFromApi(src.ID, f.Labels)
		src.Labels = labels
	}

	// ImageInfra related fields
	if f.SshPublicKey != nil {
		img.SshPublicKey = *f.SshPublicKey
	}
	if f.CertificateChain != nil {
		img.CertificateChain = *f.CertificateChain
	}

	// Proxy settings
	applyProxySettings(f.Proxy, &img)

	return img, labels
}
