package mappers

import (
	"io"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

// Label represents a simple key-value pair for form data
type Label struct {
	Key   string
	Value string
}

type SourceCreateForm struct {
	CertificateChain string
	Name             string
	HttpUrl          string
	HttpsUrl         string
	NoProxy          string
	SshPublicKey     string
	Username         string
	OrgID            string
	EmailDomain      string
	Labels           map[string]string
	IpAddress        string
	SubnetMask       string
	DefaultGateway   string
	Dns              string
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
		IpAddress:        s.IpAddress,
		SubnetMask:       s.SubnetMask,
		DefaultGateway:   s.DefaultGateway,
		Dns:              s.Dns,
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

	if s.EmailDomain != "" {
		source.EmailDomain = &s.EmailDomain
	}

	return source
}

type InventoryUpdateForm struct {
	SourceID  uuid.UUID
	AgentID   uuid.UUID
	VCenterID string
	Inventory []byte
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

func UpdateSourceFromApi(m *model.Source, vCenterID string, inventory []byte) *model.Source {
	m.Inventory = inventory
	m.VCenterID = vCenterID
	return m
}

type SourceUpdateForm struct {
	Name             *string
	Labels           []Label
	SshPublicKey     *string
	CertificateChain *string
	HttpUrl          *string
	HttpsUrl         *string
	NoProxy          *string
	IpAddress        *string
	SubnetMask       *string
	DefaultGateway   *string
	Dns              *string
}

func (f *SourceUpdateForm) ToSource(source *model.Source) {
	if f.Name != nil {
		source.Name = *f.Name
	}
}

func (f *SourceUpdateForm) ToImageInfra(imageInfra *model.ImageInfra) {
	if f.SshPublicKey != nil {
		imageInfra.SshPublicKey = *f.SshPublicKey
	}
	if f.CertificateChain != nil {
		imageInfra.CertificateChain = *f.CertificateChain
	}
	if f.HttpUrl != nil {
		imageInfra.HttpProxyUrl = *f.HttpUrl
	}
	if f.HttpsUrl != nil {
		imageInfra.HttpsProxyUrl = *f.HttpsUrl
	}
	if f.NoProxy != nil {
		imageInfra.NoProxyDomains = *f.NoProxy
	}
	if f.IpAddress != nil {
		imageInfra.IpAddress = *f.IpAddress
	}
	if f.SubnetMask != nil {
		imageInfra.SubnetMask = *f.SubnetMask
	}
	if f.DefaultGateway != nil {
		imageInfra.DefaultGateway = *f.DefaultGateway
	}
	if f.Dns != nil {
		imageInfra.Dns = *f.Dns
	}
}

func (f *SourceUpdateForm) ToLabels() []model.Label {
	labels := make([]model.Label, len(f.Labels))
	for i, label := range f.Labels {
		labels[i] = model.Label{Key: label.Key, Value: label.Value}
	}
	return labels
}

// Assessment-related mappers

type AssessmentCreateForm struct {
	ID             uuid.UUID
	Name           string
	OrgID          string
	Username       string
	OwnerFirstName *string
	OwnerLastName  *string
	Source         string
	SourceID       *uuid.UUID
	Inventory      []byte
	RVToolsFile    io.Reader
	JobID          *int64 // For rvtools source type from async job
}

func (f *AssessmentCreateForm) ToModel() model.Assessment {
	return model.Assessment{
		ID:             f.ID,
		Name:           f.Name,
		OrgID:          f.OrgID,
		Username:       f.Username,
		OwnerFirstName: f.OwnerFirstName,
		OwnerLastName:  f.OwnerLastName,
		SourceType:     f.Source,
		SourceID:       f.SourceID,
	}
}

type InventoryForm struct {
	Data v1alpha1.Inventory
}

type AssessmentUpdateForm struct {
	Name           *string
	OwnerFirstName *string
	OwnerLastName  *string
	Inventory      []byte
}
