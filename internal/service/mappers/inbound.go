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
	// Required fields (always present)
	ClusterID             string
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

type ClusterSizingForm struct {
	TotalNodes        int
	ControlPlaneNodes int
	WorkerNodes       int
	FailoverNodes     int
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
	var result v1alpha1.JobStatus
	switch state {
	case rivertype.JobStateRunning:
		switch metadata.Status {
		case model.JobStatusValidating:
			result = v1alpha1.JobStatusValidating
		case model.JobStatusCompleted:
			result = v1alpha1.JobStatusCompleted
		case model.JobStatusFailed:
			result = v1alpha1.JobStatusFailed
		case model.JobStatusCancelled:
			result = v1alpha1.JobStatusCancelled
		case model.JobStatusParsing:
			result = v1alpha1.JobStatusParsing
		default:
			result = v1alpha1.JobStatusValidating
		}

	case rivertype.JobStateCompleted:
		result = v1alpha1.JobStatusCompleted

	case rivertype.JobStateCancelled:
		result = v1alpha1.JobStatusCancelled

	case rivertype.JobStateDiscarded, rivertype.JobStateRetryable:
		result = v1alpha1.JobStatusFailed

	case rivertype.JobStateAvailable, rivertype.JobStateScheduled, rivertype.JobStatePending:
		result = v1alpha1.JobStatusPending

	default:
		result = v1alpha1.JobStatusPending
	}
	return result
}

type Mapper struct{}

func (m *Mapper) ToClusterRequirementsResponse(
	baselineResult SizingResult,
	optimizedResult *SizingResult,
	utilizationCtx UtilizationContext,
	baselineFailoverNodes int,
	optimizedFailoverNodes int,
	totalVMs int,
	totalCPU int,
	totalMemory int,
	optimizationStatus v1alpha1.OptimizationStatus,
) *v1alpha1.ClusterRequirementsResponse {
	baselineControlPlaneNodes := baselineResult.TotalNodes - baselineResult.WorkerNodes
	baselineTotalWorkers := baselineResult.WorkerNodes + baselineFailoverNodes
	baselineTotalNodes := baselineControlPlaneNodes + baselineTotalWorkers

	baseline := v1alpha1.ClusterSizing{
		TotalNodes:        baselineTotalNodes,
		WorkerNodes:       baselineTotalWorkers,
		ControlPlaneNodes: baselineControlPlaneNodes,
		FailoverNodes:     baselineFailoverNodes,
		TotalCPU:          baselineResult.TotalCPU,
		TotalMemory:       baselineResult.TotalMemory,
	}

	var optimized *v1alpha1.ClusterSizing
	var savings *v1alpha1.Savings
	if optimizedResult != nil {
		optimizedControlPlaneNodes := optimizedResult.TotalNodes - optimizedResult.WorkerNodes
		optimizedTotalWorkers := optimizedResult.WorkerNodes + optimizedFailoverNodes
		optimizedTotalNodes := optimizedControlPlaneNodes + optimizedTotalWorkers

		optimized = &v1alpha1.ClusterSizing{
			TotalNodes:           optimizedTotalNodes,
			WorkerNodes:          optimizedTotalWorkers,
			ControlPlaneNodes:    optimizedControlPlaneNodes,
			FailoverNodes:        optimizedFailoverNodes,
			TotalCPU:             optimizedResult.TotalCPU,
			TotalMemory:          optimizedResult.TotalMemory,
			CpuUtilizationMax:    &utilizationCtx.CpuPercent,
			MemoryUtilizationMax: &utilizationCtx.MemoryPercent,
			Confidence:           &utilizationCtx.Confidence,
		}

		nodesSaved := baselineTotalNodes - optimizedTotalNodes
		if nodesSaved > 0 {
			var percentageReduction float64
			if baselineTotalNodes > 0 {
				percentageReduction = (float64(nodesSaved) / float64(baselineTotalNodes)) * 100
			}

			savings = &v1alpha1.Savings{
				NodesSaved:          nodesSaved,
				PercentageReduction: percentageReduction,
				Description:         "Based on actual workload performance data",
			}
		}
	}

	resourceConsumption := v1alpha1.SizingResourceConsumption{
		Limits:          &v1alpha1.SizingResourceLimits{},
		OverCommitRatio: &v1alpha1.SizingOverCommitRatio{},
	}
	if baselineResult.ResourceConsumption != nil {
		resourceConsumption.Cpu = baselineResult.ResourceConsumption.CPU
		resourceConsumption.Memory = baselineResult.ResourceConsumption.Memory

		if baselineResult.ResourceConsumption.Limits != nil {
			resourceConsumption.Limits = &v1alpha1.SizingResourceLimits{
				Cpu:    baselineResult.ResourceConsumption.Limits.CPU,
				Memory: baselineResult.ResourceConsumption.Limits.Memory,
			}
		}

		if baselineResult.ResourceConsumption.OverCommitRatio != nil {
			resourceConsumption.OverCommitRatio = &v1alpha1.SizingOverCommitRatio{
				Cpu:    baselineResult.ResourceConsumption.OverCommitRatio.CPU,
				Memory: baselineResult.ResourceConsumption.OverCommitRatio.Memory,
			}
		}
	}

	return &v1alpha1.ClusterRequirementsResponse{
		ClusterSizing:       baseline,
		OptimizedSizing:     optimized,
		Savings:             savings,
		OptimizationStatus:  &optimizationStatus,
		ResourceConsumption: resourceConsumption,
		InventoryTotals: v1alpha1.InventoryTotals{
			TotalVMs:    totalVMs,
			TotalCPU:    totalCPU,
			TotalMemory: totalMemory,
		},
	}
}

type SizingResult struct {
	TotalNodes          int
	WorkerNodes         int
	TotalCPU            int
	TotalMemory         int
	EffectiveCPU        float64
	EffectiveMemory     float64
	ResourceConsumption *ResourceConsumption
}

type ResourceConsumption struct {
	CPU             float64
	Memory          float64
	Limits          *ResourceLimits
	OverCommitRatio *OverCommitRatio
}

type ResourceLimits struct {
	CPU    float64
	Memory float64
}

type OverCommitRatio struct {
	CPU    float64
	Memory float64
}

type UtilizationContext struct {
	CpuMultiplier    float64
	MemoryMultiplier float64
	CpuPercent       float64
	MemoryPercent    float64
	Confidence       float64
	HasData          bool
}
