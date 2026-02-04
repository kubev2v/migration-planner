package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/client"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/util"
	"github.com/kubev2v/migration-planner/pkg/log"
)

const (
	// TargetCapacityPercent leaves 30% headroom for over-commitment
	TargetCapacityPercent = 70.0
	// CapacityMultiplier is the decimal representation of TargetCapacityPercent
	CapacityMultiplier = TargetCapacityPercent / 100.0

	DefaultPlatform = "BAREMETAL"

	// MaxBatches - if exceeded, recommend larger nodes
	MaxBatches = 200

	// MinBatchCPU prevents tiny batches
	MinBatchCPU = 1.0

	// MinBatchMemory prevents tiny batches
	MinBatchMemory = 2.0

	// MaxNodeCount - if exceeded, recommend larger nodes
	MaxNodeCount = 100

	// ControlPlaneCPU is the default CPU for control plane nodes
	ControlPlaneCPU = 6
	// ControlPlaneMemory is the default memory (GB) for control plane nodes
	ControlPlaneMemory = 16

	// MachineSetNumberOfDisks is the number of disks for both worker and control plane machine sets
	MachineSetNumberOfDisks = 24

	// ControlPlaneReservedCPU is the CPU reserved for control plane services
	ControlPlaneReservedCPU = 3.5
	// ControlPlaneReservedMemory is the memory (GB) reserved for control plane services
	ControlPlaneReservedMemory = 13.39

	// MinFallbackNodeCPU is the minimum fallback CPU when inputs are invalid
	MinFallbackNodeCPU = 2
	// MinFallbackNodeMemory is the minimum fallback memory (GB) when inputs are invalid
	MinFallbackNodeMemory = 4

	// MaxRecommendedNodeCPU is the maximum recommended CPU for nodes
	MaxRecommendedNodeCPU = 200
	// MaxRecommendedNodeMemory is the maximum recommended memory (GB) for nodes
	MaxRecommendedNodeMemory = 512

	// MinFailoverNodes is the minimum number of failover nodes
	MinFailoverNodes = 2
	// FailoverCapacityPercent is the percentage of worker nodes for failover
	FailoverCapacityPercent = 10.0
)

// SizerService handles cluster sizing calculations
type SizerService struct {
	sizerClient *client.SizerClient
	store       store.Store
	logger      *log.StructuredLogger
}

// BatchedService represents an aggregated service workload for the sizer API.
// It contains the resource requirements for a single batch of VMs that will be scheduled
// as a service on the cluster.
//
// Fields:
//   - Name: Unique identifier for the batch (e.g., "vms-batch-1-services")
//   - RequiredCPU: CPU cores requested for this batch (after applying over-commit ratio)
//   - RequiredMemory: Memory (GB) requested for this batch (after applying over-commit ratio)
//   - LimitCPU: Maximum CPU cores allowed for this batch (before over-commit)
//   - LimitMemory: Maximum memory (GB) allowed for this batch (before over-commit)
//
// The Required values represent Kubernetes resource requests, while Limit values represent
// Kubernetes resource limits. The over-commit ratio determines the relationship between them.
type BatchedService struct {
	Name           string
	RequiredCPU    float64
	RequiredMemory float64
	LimitCPU       float64
	LimitMemory    float64
}

// TransformedSizerResponse represents the transformed response from the sizer service
// after mapping it to the domain model format.
type TransformedSizerResponse struct {
	ClusterSizing       mappers.ClusterSizingForm
	ResourceConsumption mappers.ResourceConsumptionForm
}

var cpuOverCommitMultipliers = map[string]float64{
	"1:1": 1.0,
	"1:2": 2.0,
	"1:4": 4.0,
	"1:6": 6.0,
}

var memoryOverCommitMultipliers = map[string]float64{
	"1:1": 1.0,
	"1:2": 2.0,
	"1:4": 4.0,
}

func NewSizerService(sizerClient *client.SizerClient, store store.Store) *SizerService {
	return &SizerService{
		sizerClient: sizerClient,
		store:       store,
		logger:      log.NewDebugLogger("sizing_service"),
	}
}

// CalculateClusterRequirements calculates cluster requirements for an assessment
func (s *SizerService) CalculateClusterRequirements(
	ctx context.Context,
	assessmentID uuid.UUID,
	req *mappers.ClusterRequirementsRequestForm,
) (*mappers.ClusterRequirementsResponseForm, error) {
	logger := s.logger.WithContext(ctx)

	if s.sizerClient == nil {
		return nil, fmt.Errorf("sizer client is not configured")
	}

	assessment, err := s.store.Assessment().Get(ctx, assessmentID)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return nil, NewErrAssessmentNotFound(assessmentID)
		}
		return nil, fmt.Errorf("failed to get assessment: %w", err)
	}

	if len(assessment.Snapshots) == 0 {
		return nil, fmt.Errorf("assessment has no snapshots")
	}

	// Store already orders snapshots by created_at DESC, so first is latest
	latestSnapshot := assessment.Snapshots[0]
	if len(latestSnapshot.Inventory) == 0 {
		return nil, fmt.Errorf("latest snapshot has empty inventory")
	}

	var inventory api.Inventory
	if err := json.Unmarshal(latestSnapshot.Inventory, &inventory); err != nil {
		return nil, fmt.Errorf("failed to parse inventory: %w", err)
	}

	// Check if inventory has any clusters
	if len(inventory.Clusters) == 0 {
		return nil, fmt.Errorf("inventory has no clusters")
	}

	clusterInventory, exists := inventory.Clusters[req.ClusterID]
	if !exists {
		return nil, fmt.Errorf("cluster %s not found in assessment %s", req.ClusterID, assessmentID)
	}

	totalVMs := clusterInventory.Vms.Total
	totalCPU := clusterInventory.Vms.CpuCores.Total
	totalMemory := clusterInventory.Vms.RamGB.Total

	// Validate that the cluster has VMs with meaningful resources to migrate
	// Reject clusters with no VMs or clusters where either CPU or Memory is zero
	if totalVMs == 0 || totalCPU == 0 || totalMemory == 0 {
		return nil, NewErrInvalidClusterInventory(req.ClusterID, "cluster has no VMs or no CPU/Memory resources and cannot be used for migration planning")
	}

	includeControlPlane := true
	controlPlaneSchedulable := req.ControlPlaneSchedulable

	controlPlaneCPU := ControlPlaneCPU
	controlPlaneMemory := ControlPlaneMemory

	// Calculate effective CPU with SMT adjustment
	effectiveCPU := CalculateEffectiveCPU(req.WorkerNodeCPU, req.WorkerNodeThreads)

	// Calculate SMT multiplier for minimum node size calculation
	smtMultiplier := 1.0 // Default: no SMT
	if req.WorkerNodeCPU > 0 && req.WorkerNodeThreads > 0 && req.WorkerNodeThreads > req.WorkerNodeCPU {
		smtMultiplier = effectiveCPU / float64(req.WorkerNodeCPU)
	}

	tracer := logger.Operation("calculate_cluster_requirements").
		WithUUID("assessment_id", assessmentID).
		WithString("cluster_id", req.ClusterID).
		WithInt("inventory_total_cpu", totalCPU).
		WithInt("inventory_total_memory", totalMemory).
		WithInt("inventory_total_vms", totalVMs).
		WithString("cpu_over_commit_ratio", req.CpuOverCommitRatio).
		WithString("memory_over_commit_ratio", req.MemoryOverCommitRatio).
		WithInt("worker_node_cpu", req.WorkerNodeCPU).
		WithInt("worker_node_threads", req.WorkerNodeThreads).
		WithString("worker_node_effective_cpu", fmt.Sprintf("%.2f", effectiveCPU)).
		WithInt("worker_node_memory", req.WorkerNodeMemory).
		WithBool("control_plane_schedulable", controlPlaneSchedulable).
		Build()

	// Use effective CPU for all calculations
	targetCPU := effectiveCPU * CapacityMultiplier
	targetMemory := float64(req.WorkerNodeMemory) * CapacityMultiplier

	minNodeCPUForMaxBatches, minNodeMemoryForMaxBatches := s.calculateMinimumNodeSize(
		totalCPU,
		totalMemory,
		MaxBatches,
		CapacityMultiplier,
		smtMultiplier,
	)

	estimatedBatchesCPU := int(math.Ceil(float64(totalCPU) / targetCPU))
	estimatedBatchesMemory := int(math.Ceil(float64(totalMemory) / targetMemory))
	estimatedBatches := max(estimatedBatchesCPU, estimatedBatchesMemory)

	if estimatedBatches > MaxBatches {
		return nil, s.formatNodeSizeError(
			req.WorkerNodeCPU, req.WorkerNodeMemory,
			totalCPU, totalMemory,
			minNodeCPUForMaxBatches, minNodeMemoryForMaxBatches,
		)
	}

	services, err := s.aggregateVMsIntoServices(
		float64(totalCPU),
		float64(totalMemory),
		effectiveCPU,
		req.WorkerNodeMemory,
		req.CpuOverCommitRatio,
		req.MemoryOverCommitRatio,
		CapacityMultiplier,
	)
	if err != nil {
		return nil, NewErrInvalidRequest(err.Error())
	}

	tracer.Step("batching_complete").
		WithInt("num_services", len(services)).
		Log()

	// Build sizer API payload
	sizerPayload := s.buildSizerPayload(
		services,
		DefaultPlatform,
		effectiveCPU,
		req.WorkerNodeMemory,
		includeControlPlane,
		controlPlaneSchedulable,
		controlPlaneCPU,
		controlPlaneMemory,
	)

	// Call sizer service
	sizerResponse, err := s.sizerClient.CalculateSizing(ctx, sizerPayload)
	if err != nil {
		tracer.Error(err).Log()
		return nil, fmt.Errorf("failed to call sizer service: %w", err)
	}
	if sizerResponse == nil {
		return nil, fmt.Errorf("sizer service returned empty response")
	}

	transformed := s.transformSizerResponse(sizerResponse)

	if transformed.ClusterSizing.TotalNodes > MaxNodeCount {
		minNodeCPU, minNodeMemory := s.calculateMinimumNodeSize(
			totalCPU,
			totalMemory,
			MaxNodeCount,
			CapacityMultiplier,
			smtMultiplier,
		)

		return nil, s.formatNodeSizeError(
			req.WorkerNodeCPU, req.WorkerNodeMemory,
			totalCPU, totalMemory,
			minNodeCPU, minNodeMemory,
		)
	}

	tracer.Success().
		WithInt("total_nodes", transformed.ClusterSizing.TotalNodes).
		Log()

	return &mappers.ClusterRequirementsResponseForm{
		ClusterSizing:       transformed.ClusterSizing,
		ResourceConsumption: transformed.ResourceConsumption,
		InventoryTotals: mappers.InventoryTotalsForm{
			TotalVMs:    totalVMs,
			TotalCPU:    totalCPU,
			TotalMemory: totalMemory,
		},
	}, nil
}

// Health checks if the sizer service is healthy
func (s *SizerService) Health(ctx context.Context) error {
	if s.sizerClient == nil {
		return fmt.Errorf("sizer client is not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.sizerClient.HealthCheck(ctx)
}

// aggregateVMsIntoServices implements the batching algorithm that splits the total VM resources
// (CPU and memory) from a cluster into multiple service batches for the sizer API.
//
// The algorithm:
//  1. Calculates the number of batches needed based on worker node capacity (using 70% capacity multiplier)
//  2. Distributes total CPU and memory evenly across all batches
//  3. Enforces minimum batch sizes (MinBatchCPU, MinBatchMemory) to prevent tiny batches
//  4. Applies the over-commit ratio to both CPU and memory to calculate required resources (requests) vs limit resources
//
// Each batch becomes a BatchedService that represents a service workload to be scheduled on the cluster.
// These batched services are later converted into the sizer API format via buildSizerPayload.
//
// Parameters:
//   - totalCPU: Total CPU cores from all VMs in the cluster
//   - totalMemory: Total memory (GB) from all VMs in the cluster
//   - effectiveWorkerNodeCPU: Effective CPU cores per worker node (SMT-adjusted)
//   - workerNodeMemory: Memory (GB) per worker node
//   - cpuOverCommitRatio: CPU over-commit ratio (e.g., "1:4")
//   - memoryOverCommitRatio: Memory over-commit ratio (e.g., "1:2")
//   - capacityMultiplier: Multiplier for target capacity (0.7 for 70% utilization)
//
// Returns:
//   - []BatchedService: Array of batched services, one per batch
//   - error: Error if over-commit ratio is invalid or other processing error occurs
func (s *SizerService) aggregateVMsIntoServices(
	totalCPU float64,
	totalMemory float64,
	effectiveWorkerNodeCPU float64,
	workerNodeMemory int,
	cpuOverCommitRatio string,
	memoryOverCommitRatio string,
	capacityMultiplier float64,
) ([]BatchedService, error) {
	// Guard against invalid worker node sizes to prevent division by zero
	if effectiveWorkerNodeCPU <= 0 || workerNodeMemory <= 0 {
		return nil, fmt.Errorf("worker node size must be greater than zero: CPU=%.2f, Memory=%d", effectiveWorkerNodeCPU, workerNodeMemory)
	}

	// Calculate target batch size (70% of node capacity)
	targetCPU := effectiveWorkerNodeCPU * capacityMultiplier
	targetMemory := float64(workerNodeMemory) * capacityMultiplier

	// Calculate number of batches
	batchesCPU := int(math.Ceil(totalCPU / targetCPU))
	batchesMemory := int(math.Ceil(totalMemory / targetMemory))
	numBatches := max(batchesCPU, batchesMemory)

	if numBatches < 1 {
		numBatches = 1
	}

	// Distribute resources evenly across batches
	cpuPerBatch := totalCPU / float64(numBatches)
	memoryPerBatch := totalMemory / float64(numBatches)

	// Enforce minimum batch size
	if cpuPerBatch < MinBatchCPU || memoryPerBatch < MinBatchMemory {
		batchesFromMinCPU := int(math.Ceil(totalCPU / MinBatchCPU))
		batchesFromMinMemory := int(math.Ceil(totalMemory / MinBatchMemory))
		numBatches = max(batchesFromMinCPU, batchesFromMinMemory)

		if numBatches < 1 {
			numBatches = 1
		}

		cpuPerBatch = totalCPU / float64(numBatches)
		memoryPerBatch = totalMemory / float64(numBatches)
	}

	// Calculate requests and limits
	limitCPU := cpuPerBatch
	limitMemory := memoryPerBatch

	cpuOverCommitMultiplier, err := s.getCpuOverCommitMultiplier(cpuOverCommitRatio)
	if err != nil {
		return nil, err
	}
	memoryOverCommitMultiplier, err := s.getMemoryOverCommitMultiplier(memoryOverCommitRatio)
	if err != nil {
		return nil, err
	}
	requiredCPU := limitCPU / cpuOverCommitMultiplier
	requiredMemory := limitMemory / memoryOverCommitMultiplier

	services := make([]BatchedService, numBatches)
	for i := 0; i < numBatches; i++ {
		services[i] = BatchedService{
			Name:           fmt.Sprintf("vms-batch-%d-services", i+1),
			RequiredCPU:    requiredCPU,
			RequiredMemory: requiredMemory,
			LimitCPU:       limitCPU,
			LimitMemory:    limitMemory,
		}
	}

	return services, nil
}

// getCpuOverCommitMultiplier converts CPU overcommit ratio string to multiplier.
func (s *SizerService) getCpuOverCommitMultiplier(cpuOverCommitRatio string) (float64, error) {
	multiplier, ok := cpuOverCommitMultipliers[cpuOverCommitRatio]
	if !ok {
		err := fmt.Errorf("unknown CPU over-commit ratio %q", cpuOverCommitRatio)
		s.logger.Operation("get_cpu_over_commit_multiplier").
			WithString("cpu_over_commit_ratio", cpuOverCommitRatio).
			Build().
			Error(err).
			Log()
		return 0, err
	}
	return multiplier, nil
}

// getMemoryOverCommitMultiplier converts memory overcommit ratio string to multiplier.
func (s *SizerService) getMemoryOverCommitMultiplier(memoryOverCommitRatio string) (float64, error) {
	multiplier, ok := memoryOverCommitMultipliers[memoryOverCommitRatio]
	if !ok {
		err := fmt.Errorf("unknown memory over-commit ratio %q", memoryOverCommitRatio)
		s.logger.Operation("get_memory_over_commit_multiplier").
			WithString("memory_over_commit_ratio", memoryOverCommitRatio).
			Build().
			Error(err).
			Log()
		return 0, err
	}
	return multiplier, nil
}

// CalculateEffectiveCPU calculates SMT-adjusted effective CPU cores
// Formula: effectiveCPU = physicalCores + ((threads - physicalCores) * 0.5)
// If threads == 0 or threads == cores, returns cores (no SMT)
func CalculateEffectiveCPU(physicalCores, threads int) float64 {
	if physicalCores <= 0 {
		return 0.0 // Invalid input, return 0
	}
	if threads == 0 || threads == physicalCores {
		return float64(physicalCores) // No SMT or not provided
	}
	if threads < physicalCores {
		// Invalid configuration, return cores as fallback
		return float64(physicalCores)
	}
	smtThreads := float64(threads - physicalCores)
	return float64(physicalCores) + (smtThreads * 0.5)
}

func (s *SizerService) calculateMinimumNodeSize(inventoryCPU, inventoryMemory int, maxCount int, capacityMultiplier float64, smtMultiplier float64) (cpu, memory int) {
	// Validate inputs to prevent division by zero
	if maxCount <= 0 || capacityMultiplier <= 0 || smtMultiplier <= 0 {
		// Return minimum valid node size if inputs are invalid
		return MinFallbackNodeCPU, MinFallbackNodeMemory
	}

	// Calculate minimum effective CPU needed per node
	denominator := float64(maxCount) * capacityMultiplier
	minEffectiveCPUPerNode := float64(inventoryCPU) / denominator

	// Convert effective CPU back to physical cores (accounting for SMT)
	minNodeCPU := int(math.Ceil(minEffectiveCPUPerNode / smtMultiplier))
	minNodeMemory := int(math.Ceil(float64(inventoryMemory) / denominator))

	// Round up to nearest even number for CPU, nearest multiple of 4 for memory
	minNodeCPU = int(math.Ceil(float64(minNodeCPU)/2) * 2)
	minNodeMemory = int(math.Ceil(float64(minNodeMemory)/4) * 4)

	if minNodeCPU < MinFallbackNodeCPU {
		minNodeCPU = MinFallbackNodeCPU
	}
	if minNodeMemory < MinFallbackNodeMemory {
		minNodeMemory = MinFallbackNodeMemory
	}

	// Cap at maximums
	if minNodeCPU > MaxRecommendedNodeCPU {
		minNodeCPU = MaxRecommendedNodeCPU
	}
	if minNodeMemory > MaxRecommendedNodeMemory {
		minNodeMemory = MaxRecommendedNodeMemory
	}

	return minNodeCPU, minNodeMemory
}

// formatNodeSizeError creates a standardized error message for node size validation failures
func (s *SizerService) formatNodeSizeError(workerCPU, workerMemory, inventoryCPU, inventoryMemory, minCPU, minMemory int) error {
	message := fmt.Sprintf(
		"worker node size (%d CPU / %d GB) is too small for this inventory (%d CPU / %d GB). "+
			"Please use larger worker nodes (recommended: at least %d CPU / %d GB)",
		workerCPU, workerMemory,
		inventoryCPU, inventoryMemory,
		minCPU, minMemory,
	)
	return NewErrInvalidRequest(message)
}

// buildSizerPayload transforms batched services into sizer API format
func (s *SizerService) buildSizerPayload(
	services []BatchedService,
	platform string,
	effectiveWorkerNodeCPU float64,
	workerNodeMemory int,
	includeControlPlane bool,
	controlPlaneSchedulable bool,
	controlPlaneCPU int,
	controlPlaneMemory int,
) *client.SizerRequest {
	machineSets := []client.MachineSet{
		{
			Name:          "worker",
			CPU:           int(math.Round(effectiveWorkerNodeCPU)),
			Memory:        workerNodeMemory,
			InstanceName:  "bare-metal-worker",
			NumberOfDisks: MachineSetNumberOfDisks,
			OnlyFor:       []string{},
			Label:         "Worker",
		},
	}

	if includeControlPlane {
		machineSets = append(machineSets, client.MachineSet{
			Name:                    "controlPlane",
			CPU:                     controlPlaneCPU,
			Memory:                  controlPlaneMemory,
			InstanceName:            "control-plane",
			NumberOfDisks:           MachineSetNumberOfDisks,
			OnlyFor:                 []string{},
			Label:                   "Control Plane",
			AllowWorkloadScheduling: util.BoolPtr(controlPlaneSchedulable),
			ControlPlaneReserved: &client.ControlPlaneReserved{
				CPU:    ControlPlaneReservedCPU,
				Memory: ControlPlaneReservedMemory,
			},
		})
	}

	vmWorkload := client.Workload{
		Name:         "vm-workload",
		Count:        1,
		UsesMachines: []string{"worker"},
		Services:     make([]client.ServiceDescriptor, len(services)),
	}

	var workloads []client.Workload

	if includeControlPlane {
		workloads = []client.Workload{
			{
				Name:         "control-plane-services",
				Count:        3,
				UsesMachines: []string{"controlPlane"},
				Services: []client.ServiceDescriptor{
					{
						Name:           "ControlPlane",
						RequiredCPU:    ControlPlaneReservedCPU,
						RequiredMemory: ControlPlaneReservedMemory,
						Zones:          1,
						RunsWith:       []string{},
						Avoid:          []string{},
					},
				},
			},
			vmWorkload,
		}
	} else {
		workloads = []client.Workload{vmWorkload}
	}

	// VM workload is always last in the array
	vmWorkloadIndex := len(workloads) - 1

	if controlPlaneSchedulable && includeControlPlane {
		workloads[vmWorkloadIndex].UsesMachines = []string{"worker", "controlPlane"}
	}

	for i, svc := range services {
		workloads[vmWorkloadIndex].Services[i] = client.ServiceDescriptor{
			Name:           svc.Name,
			RequiredCPU:    svc.RequiredCPU,
			RequiredMemory: svc.RequiredMemory,
			LimitCPU:       svc.LimitCPU,
			LimitMemory:    svc.LimitMemory,
			Zones:          1,
			RunsWith:       []string{},
			Avoid:          []string{},
		}
	}

	return &client.SizerRequest{
		Platform:    platform,
		MachineSets: machineSets,
		Workloads:   workloads,
		Detailed:    true,
	}
}

// transformSizerResponse maps sizer service response to API response
func (s *SizerService) transformSizerResponse(sizerResponse *client.SizerResponse) TransformedSizerResponse {
	totalNodes := sizerResponse.Data.NodeCount
	workerNodes := 0
	controlPlaneNodes := 0

	if len(sizerResponse.Data.Advanced) > 0 {
		for _, zone := range sizerResponse.Data.Advanced {
			for _, node := range zone.Nodes {
				if node.IsControlPlane {
					controlPlaneNodes++
				} else {
					workerNodes++
				}
			}
		}
	} else {
		if totalNodes >= 3 {
			controlPlaneNodes = 3
			workerNodes = totalNodes - 3
		} else {
			workerNodes = totalNodes
		}
	}

	// Add failover capacity: max(2 nodes, 10% of worker nodes)
	failoverNodes := calculateFailoverNodes(workerNodes)
	workerNodes += failoverNodes
	totalNodes += failoverNodes

	resourceConsumptionForm := mappers.ResourceConsumptionForm{
		CPU:    sizerResponse.Data.ResourceConsumption.CPU,
		Memory: sizerResponse.Data.ResourceConsumption.Memory,
		Limits: mappers.ResourceLimitsForm{
			CPU:    0.0,
			Memory: 0.0,
		},
		OverCommitRatio: mappers.OverCommitRatioForm{
			CPU:    0.0,
			Memory: 0.0,
		},
	}

	if sizerResponse.Data.ResourceConsumption.Limits != nil {
		resourceConsumptionForm.Limits = mappers.ResourceLimitsForm{
			CPU:    sizerResponse.Data.ResourceConsumption.Limits.CPU,
			Memory: sizerResponse.Data.ResourceConsumption.Limits.Memory,
		}
	}

	if sizerResponse.Data.ResourceConsumption.OverCommitRatio != nil {
		resourceConsumptionForm.OverCommitRatio = mappers.OverCommitRatioForm{
			CPU:    sizerResponse.Data.ResourceConsumption.OverCommitRatio.CPU,
			Memory: sizerResponse.Data.ResourceConsumption.OverCommitRatio.Memory,
		}
	}

	return TransformedSizerResponse{
		ClusterSizing: mappers.ClusterSizingForm{
			TotalNodes:        totalNodes,
			WorkerNodes:       workerNodes,
			ControlPlaneNodes: controlPlaneNodes,
			FailoverNodes:     failoverNodes,
			TotalCPU:          sizerResponse.Data.TotalCPU,
			TotalMemory:       sizerResponse.Data.TotalMemory,
		},
		ResourceConsumption: resourceConsumptionForm,
	}
}

func calculateFailoverNodes(workerNodes int) int {
	if workerNodes == 0 {
		return 0
	}
	percentageBased := int(math.Ceil(float64(workerNodes) * FailoverCapacityPercent / 100.0))
	return max(MinFailoverNodes, percentageBased)
}
