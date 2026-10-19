package mappers

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	svcmodel "github.com/kubev2v/migration-planner/internal/service/model"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

// Label represents a simple key-value pair for form data
type Label struct {
	Key   string
	Value string
}

type SourceCreateForm struct {
	CertificateChain  string
	Name              string
	HttpUrl           string
	HttpsUrl          string
	NoProxy           string
	SshPublicKey      string
	Username          string
	OrgID             string
	EmailDomain       string
	Labels            map[string]string
	IpAddress         string
	SubnetMask        string
	DefaultGateway    string
	Dns               string
	EnableProxy       *bool
	NetworkConfigType *string
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
	if s.EnableProxy != nil && !*s.EnableProxy {
		imageInfra.HttpProxyUrl = ""
		imageInfra.HttpsProxyUrl = ""
		imageInfra.NoProxyDomains = ""
	}
	if s.NetworkConfigType != nil && *s.NetworkConfigType == "dhcp" {
		imageInfra.IpAddress = ""
		imageInfra.SubnetMask = ""
		imageInfra.DefaultGateway = ""
		imageInfra.Dns = ""
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
	// Subsets contains the subset inventories to replace.
	// SourceService.UpdateInventory replaces all existing subsets with this value.
	// Nil or empty input clears all subsets.
	// Note: AgentService.UpdateSourceInventory ignores this field.
	Subsets []SourceSubsetUpdateForm
}

type SourceInventoryUpdateForm struct {
	SourceID  uuid.UUID
	VCenterID string
	Inventory []byte
}

type SourceSubsetUpdateForm struct {
	ID        uuid.UUID
	Name      string
	SourceID  uuid.UUID
	VCenterID string
	VMsCount  int
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
	Name              *string
	Labels            []Label
	SshPublicKey      *string
	CertificateChain  *string
	HttpUrl           *string
	HttpsUrl          *string
	NoProxy           *string
	IpAddress         *string
	SubnetMask        *string
	DefaultGateway    *string
	Dns               *string
	EnableProxy       *bool
	NetworkConfigType *string
}

func (f *SourceUpdateForm) ToSource(source *model.Source) {
	if f.Name != nil {
		source.Name = *f.Name
	}
}

func (f *SourceUpdateForm) ToImageInfra(imageInfra *model.ImageInfra) {
	if f.EnableProxy != nil && !*f.EnableProxy {
		imageInfra.HttpProxyUrl = ""
		imageInfra.HttpsProxyUrl = ""
		imageInfra.NoProxyDomains = ""
	}
	if f.NetworkConfigType != nil && *f.NetworkConfigType == "dhcp" {
		imageInfra.IpAddress = ""
		imageInfra.SubnetMask = ""
		imageInfra.DefaultGateway = ""
		imageInfra.Dns = ""
	}
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
	if f.Labels == nil {
		return nil
	}
	labels := make([]model.Label, len(f.Labels))
	for i, label := range f.Labels {
		labels[i] = model.Label{Key: label.Key, Value: label.Value}
	}
	return labels
}

// Assessment-related mappers

type AssessmentCreateForm struct {
	ID             uuid.UUID
	SourceType     string
	Name           string
	OrgID          string
	Username       string
	OwnerFirstName *string
	OwnerLastName  *string
	SourceID       *uuid.UUID
	Inventory      svcmodel.AssessmentInventory
}

func (f *AssessmentCreateForm) ToModel() model.Assessment {
	return model.Assessment{
		ID:             f.ID,
		Name:           f.Name,
		OrgID:          f.OrgID,
		Username:       f.Username,
		OwnerFirstName: f.OwnerFirstName,
		OwnerLastName:  f.OwnerLastName,
		SourceType:     f.SourceType,
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

type ClusterRequirementsRequestForm struct {
	// Required fields (always present)
	ClusterID string

	// ClusterIDs is an internal future multi-cluster selection.
	// Nil uses ClusterID for compatibility; an empty list is invalid.
	ClusterIDs            []string
	CpuOverCommitRatio    string
	MemoryOverCommitRatio string
	WorkerNodeCPU         int
	WorkerNodeMemory      int

	// Optional fields (use pointers to distinguish omitted vs zero-value)
	WorkerNodeThreads       *int
	ControlPlaneSchedulable *bool
	ControlPlaneNodeCount   *int
	ControlPlaneCPU         *int
	ControlPlaneMemory      *int
	HostedControlPlane      *bool
	CompactMode             *bool
}

type ClusterRequirementsInputForm struct {
	ClusterID               string
	CpuOverCommitRatio      *string
	MemoryOverCommitRatio   *string
	WorkerNodeCPU           *int
	WorkerNodeThreads       *int
	WorkerNodeMemory        *int
	ControlPlaneSchedulable *bool
	ControlPlaneNodeCount   *int
	ControlPlaneCPU         *int
	ControlPlaneMemory      *int
	HostedControlPlane      *bool
	CompactMode             *bool
}

// AssessmentInventoryFromInventory builds a service snapshot from structured inventory.
func AssessmentInventoryFromInventory(id, assessmentID uuid.UUID, name string, isSubset bool, inventory svcmodel.Inventory) svcmodel.AssessmentInventory {
	if inventory.Clusters == nil {
		inventory.Clusters = make(map[string]svcmodel.InventoryData)
	}
	result := svcmodel.AssessmentInventory{
		ID:           id,
		Name:         name,
		IsSubset:     isSubset,
		VCenterID:    inventory.VcenterId,
		Version:      svcmodel.InventoryVersionV2,
		Inventory:    inventory,
		AssessmentID: assessmentID,
	}
	if inventory.Vcenter != nil {
		result.VMsCount = inventory.Vcenter.Vms.Total
		result.HostsCount = inventory.Vcenter.Infra.TotalHosts
		result.NetworksCount = len(inventory.Vcenter.Infra.Networks)
		result.DatastoresCount = len(inventory.Vcenter.Infra.Datastores)
	}
	return result
}

// AssessmentInventoryToStore converts the domain inventory snapshot to the
// persistence model, marshaling its structured Inventory into a jsonb blob.
func AssessmentInventoryToStore(ai svcmodel.AssessmentInventory) (model.AssessmentInventory, error) {
	raw, err := json.Marshal(ai.Inventory)
	if err != nil {
		return model.AssessmentInventory{}, fmt.Errorf("failed to marshal inventory: %w", err)
	}
	return model.AssessmentInventory{
		ID:              ai.ID,
		CreatedAt:       ai.CreatedAt,
		Name:            ai.Name,
		IsSubset:        ai.IsSubset,
		VCenterID:       ai.VCenterID,
		VMsCount:        ai.VMsCount,
		HostsCount:      ai.HostsCount,
		NetworksCount:   ai.NetworksCount,
		DatastoresCount: ai.DatastoresCount,
		Version:         ai.Version,
		Inventory:       raw,
		AssessmentID:    ai.AssessmentID,
	}, nil
}
