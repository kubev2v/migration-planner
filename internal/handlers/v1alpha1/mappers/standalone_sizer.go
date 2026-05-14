package mappers

import (
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
)

func StandaloneClusterRequirementsRequestToForm(
	apiReq api.StandaloneClusterRequirementsRequest,
) mappers.StandaloneClusterRequirementsRequestForm {
	form := mappers.StandaloneClusterRequirementsRequestForm{
		TotalVMs:                apiReq.TotalVMs,
		TotalCPU:                apiReq.TotalCPU,
		TotalMemory:             apiReq.TotalMemory,
		CpuOverCommitRatio:      string(apiReq.CpuOverCommitRatio),
		MemoryOverCommitRatio:   string(apiReq.MemoryOverCommitRatio),
		WorkerNodeCPU:           apiReq.WorkerNodeCPU,
		WorkerNodeMemory:        apiReq.WorkerNodeMemory,
		WorkerNodeThreads:       apiReq.WorkerNodeThreads,
		HostedControlPlane:      apiReq.HostedControlPlane,
		ControlPlaneSchedulable: apiReq.ControlPlaneSchedulable,
		ControlPlaneCPU:         apiReq.ControlPlaneCPU,
		ControlPlaneMemory:      apiReq.ControlPlaneMemory,
	}

	if apiReq.ControlPlaneNodeCount != nil {
		nodeCount := int(*apiReq.ControlPlaneNodeCount)
		form.ControlPlaneNodeCount = &nodeCount
	}

	return form
}

func StandaloneClusterRequirementsResponseFormToAPI(
	form mappers.StandaloneClusterRequirementsResponseForm,
) api.StandaloneClusterRequirementsResponse {
	resourceConsumption := api.SizingResourceConsumption{
		Cpu:    form.ResourceConsumption.CPU,
		Memory: form.ResourceConsumption.Memory,
	}

	if form.ResourceConsumption.Limits.CPU != 0.0 || form.ResourceConsumption.Limits.Memory != 0.0 {
		resourceConsumption.Limits = &api.SizingResourceLimits{
			Cpu:    form.ResourceConsumption.Limits.CPU,
			Memory: form.ResourceConsumption.Limits.Memory,
		}
	}

	if form.ResourceConsumption.OverCommitRatio.CPU != 0.0 || form.ResourceConsumption.OverCommitRatio.Memory != 0.0 {
		resourceConsumption.OverCommitRatio = &api.SizingOverCommitRatio{
			Cpu:    form.ResourceConsumption.OverCommitRatio.CPU,
			Memory: form.ResourceConsumption.OverCommitRatio.Memory,
		}
	}

	return api.StandaloneClusterRequirementsResponse{
		ClusterSizing: api.ClusterSizing{
			TotalNodes:        form.ClusterSizing.TotalNodes,
			ControlPlaneNodes: form.ClusterSizing.ControlPlaneNodes,
			WorkerNodes:       form.ClusterSizing.WorkerNodes,
			FailoverNodes:     form.ClusterSizing.FailoverNodes,
			TotalCPU:          form.ClusterSizing.TotalCPU,
			TotalMemory:       form.ClusterSizing.TotalMemory,
		},
		ResourceConsumption: resourceConsumption,
	}
}
