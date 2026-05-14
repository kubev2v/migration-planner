package mappers

// StandaloneClusterRequirementsRequestForm is for hypothetical sizing with inline inventory data.
type StandaloneClusterRequirementsRequestForm struct {
	TotalVMs    int
	TotalCPU    int
	TotalMemory int

	CpuOverCommitRatio      string
	MemoryOverCommitRatio   string
	WorkerNodeCPU           int
	WorkerNodeMemory        int
	WorkerNodeThreads       *int
	ControlPlaneSchedulable *bool
	ControlPlaneNodeCount   *int
	ControlPlaneCPU         *int
	ControlPlaneMemory      *int
	HostedControlPlane      *bool
}

// StandaloneClusterRequirementsResponseForm omits inventory totals (already in request).
type StandaloneClusterRequirementsResponseForm struct {
	ClusterSizing       ClusterSizingForm
	ResourceConsumption ResourceConsumptionForm
}
