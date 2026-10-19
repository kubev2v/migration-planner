package mappers

import (
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/riverqueue/river/rivertype"
)

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

// StandaloneClusterRequirementsResponseForm omits inventory totals (already in request).
type StandaloneClusterRequirementsResponseForm struct {
	ClusterSizing       ClusterSizingForm
	ResourceConsumption ResourceConsumptionForm
}
