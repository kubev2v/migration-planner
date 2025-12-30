package mappers

import (
	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/riverqueue/river/rivertype"
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

// Job-related mappers

// JobForm wraps data needed to create an API Job
type JobForm struct {
	ID       int64
	State    rivertype.JobState
	Metadata model.RVToolsJobMetadata
}

// ToAPIJob converts JobForm to API Job
func (f *JobForm) ToAPIJob() *v1alpha1.Job {
	status := mapRiverStateToStatus(f.State, f.Metadata)

	job := &v1alpha1.Job{
		Id:     f.ID,
		Status: status,
	}

	if f.Metadata.Error != "" {
		job.Error = &f.Metadata.Error
	}

	if f.Metadata.AssessmentID != nil {
		job.AssessmentId = f.Metadata.AssessmentID
	}

	return job
}

type ClusterRequirementsRequestForm struct {
	ClusterID               string
	OverCommitRatio         string
	WorkerNodeCPU           int
	WorkerNodeMemory        int
	ControlPlaneSchedulable bool
}

type ClusterRequirementsResponseForm struct {
	ClusterSizing       ClusterSizingForm
	ResourceConsumption ResourceConsumptionForm
	InventoryTotals     InventoryTotalsForm
}

type ClusterSizingForm struct {
	TotalNodes        int
	ControlPlaneNodes int
	WorkerNodes       int
	TotalCPU          int
	TotalMemory       int
}

type ResourceConsumptionForm struct {
	CPU             float64
	Memory          float64
	Limits          ResourceLimitsForm
	OverCommitRatio OverCommitRatioForm
}

type ResourceLimitsForm struct {
	CPU    float64
	Memory float64
}

type OverCommitRatioForm struct {
	CPU    float64
	Memory float64
}

type InventoryTotalsForm struct {
	TotalVMs    int
	TotalCPU    int
	TotalMemory int
}

func mapRiverStateToStatus(state rivertype.JobState, metadata model.RVToolsJobMetadata) v1alpha1.JobStatus {
	switch state {
	case rivertype.JobStateRunning:
		switch metadata.Status {
		case model.JobStatusValidating:
			return v1alpha1.Validating
		case model.JobStatusCompleted:
			return v1alpha1.Completed
		case model.JobStatusFailed:
			return v1alpha1.Failed
		case model.JobStatusCancelled:
			return v1alpha1.Cancelled
		case model.JobStatusParsing:
			return v1alpha1.Parsing
		default:
			return v1alpha1.Parsing
		}

	case rivertype.JobStateCompleted:
		return v1alpha1.Completed

	case rivertype.JobStateCancelled:
		return v1alpha1.Cancelled

	case rivertype.JobStateDiscarded, rivertype.JobStateRetryable:
		return v1alpha1.Failed

	case rivertype.JobStateAvailable, rivertype.JobStateScheduled, rivertype.JobStatePending:
		return v1alpha1.Pending

	default:
		return v1alpha1.Pending
	}
}
