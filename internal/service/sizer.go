package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
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
	// TargetCapacityPercent leaves 20% headroom for over-commitment
	TargetCapacityPercent = 80.0
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

	// MaxVMsPerWorkerNode - maximum VMs allowed per worker node
	MaxVMsPerWorkerNode = 200

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
	VMCount        int // Number of VMs in this batch
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

	includeControlPlane := !req.HostedControlPlane
	effectiveCPNodeCount := effectiveControlPlaneNodeCount(req)
	controlPlaneSchedulable := req.ControlPlaneSchedulable

	controlPlaneCPU := req.ControlPlaneCPU
	controlPlaneMemory := req.ControlPlaneMemory

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
		WithBool("hosted_control_plane", req.HostedControlPlane).
		WithInt("control_plane_node_count", effectiveCPNodeCount).
		Build()

	// Use effectiveControlPlaneNodeCount to handle hosted control plane (returns 0)
	singleNode := effectiveControlPlaneNodeCount(req) == 1

	// Early validation for multi-node clusters only (SNO validated by sizer API)
	if !singleNode {
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
	}

	services, err := s.aggregateVMsIntoServices(
		float64(totalCPU),
		float64(totalMemory),
		totalVMs,
		effectiveCPU,
		req.WorkerNodeMemory,
		req.CpuOverCommitRatio,
		req.MemoryOverCommitRatio,
		CapacityMultiplier,
	)
	if err != nil {
		// For SNO, convert technical errors to user-friendly message
		if singleNode {
			return nil, s.singleNodeFitError(totalCPU, totalMemory, smtMultiplier, req.ControlPlaneCPU, req.ControlPlaneMemory)
		}
		return nil, NewErrInvalidRequest(err.Error())
	}

	tracer.Step("batching_complete").
		WithInt("num_services", len(services)).
		Log()

	// Single-node clusters must have schedulable control planes
	if singleNode && !controlPlaneSchedulable {
		return nil, NewErrInvalidRequest(
			"single-node clusters require schedulable control planes. " +
				"Set ControlPlaneSchedulable to true or use multiple control plane nodes",
		)
	}

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
		effectiveCPNodeCount,
		singleNode,
	)

	// Call sizer service
	sizerResponse, err := s.sizerClient.CalculateSizing(ctx, sizerPayload)
	if err != nil {
		tracer.Error(err).Log()
		if singleNode && isSizerSchedulabilityError(err) {
			return nil, s.singleNodeFitError(totalCPU, totalMemory, smtMultiplier, req.ControlPlaneCPU, req.ControlPlaneMemory)
		}
		return nil, fmt.Errorf("failed to call sizer service: %w", err)
	}
	if sizerResponse == nil {
		return nil, fmt.Errorf("sizer service returned empty response")
	}

	// Create service-to-VM-count mapping for tracking VMs per node
	serviceToVMCount := make(map[string]int)
	for _, svc := range services {
		serviceToVMCount[svc.Name] = svc.VMCount
	}

	transformed := s.transformSizerResponse(sizerResponse, effectiveCPNodeCount)

	if singleNode && sizerResponse.Data.NodeCount > 1 {
		return nil, s.singleNodeFitError(totalCPU, totalMemory, smtMultiplier, req.ControlPlaneCPU, req.ControlPlaneMemory)
	}

	// Calculate max VMs per node from sizer response for validation. We only validate when
	// Advanced is present; the sizer is called with Detailed: true and is expected to return
	// a complete placement there. Multi-node: worker nodes only. Single-node: control plane
	// hosts workloads, so include control plane nodes in the scan.
	maxVMsPerNode := 0
	if len(sizerResponse.Data.Advanced) > 0 {
		for _, zone := range sizerResponse.Data.Advanced {
			for _, node := range zone.Nodes {
				if node.IsControlPlane && !singleNode {
					continue
				}
				vmsOnNode := 0
				for _, serviceName := range node.Services {
					if vmCount, exists := serviceToVMCount[serviceName]; exists {
						vmsOnNode += vmCount
					}
				}
				if vmsOnNode > maxVMsPerNode {
					maxVMsPerNode = vmsOnNode
				}
			}
		}
	}

	if maxVMsPerNode > MaxVMsPerWorkerNode {
		if singleNode {
			return nil, s.singleNodeFitError(totalCPU, totalMemory, smtMultiplier, req.ControlPlaneCPU, req.ControlPlaneMemory)
		}
		err := NewErrInvalidRequest(fmt.Sprintf("VM distribution constraint violated: found %d VMs on a node, exceeds limit of %d per node",
			maxVMsPerNode, MaxVMsPerWorkerNode))
		s.logger.Operation("vm_limit_exceeded").
			WithInt("max_vms_per_node", maxVMsPerNode).
			WithInt("limit", MaxVMsPerWorkerNode).
			Build().
			Error(err).
			Log()
		return nil, err
	}

	tracer.Step("sizer_results").
		WithInt("max_vms_per_node", maxVMsPerNode).
		WithInt("total_nodes", transformed.ClusterSizing.TotalNodes).
		WithInt("num_services", len(services)).
		Log()
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
//  1. Calculates the number of batches needed based on worker node capacity (using 80% capacity multiplier)
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
//   - totalVMs: Total number of VMs in the cluster
//   - effectiveWorkerNodeCPU: Effective CPU cores per worker node (SMT-adjusted)
//   - workerNodeMemory: Memory (GB) per worker node
//   - cpuOverCommitRatio: CPU over-commit ratio (e.g., "1:4")
//   - memoryOverCommitRatio: Memory over-commit ratio (e.g., "1:2")
//   - capacityMultiplier: Multiplier for target capacity (0.8 for 80% utilization)
//
// Returns:
//   - []BatchedService: Array of batched services, one per batch
//   - error: Error if over-commit ratio is invalid or other processing error occurs
func (s *SizerService) aggregateVMsIntoServices(
	totalCPU float64,
	totalMemory float64,
	totalVMs int,
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

	if totalVMs == 0 {
		return []BatchedService{}, nil
	}

	// Calculate target batch size (80% of node capacity)
	targetCPU := effectiveWorkerNodeCPU * capacityMultiplier
	targetMemory := float64(workerNodeMemory) * capacityMultiplier

	// Calculate number of batches based on resources
	batchesCPU := int(math.Ceil(totalCPU / targetCPU))
	batchesMemory := int(math.Ceil(totalMemory / targetMemory))

	// Ensure each batch has at most MaxVMsPerWorkerNode VMs
	minBatchesFromVMLimit := 1
	if totalVMs > 0 && MaxVMsPerWorkerNode > 0 {
		minBatchesFromVMLimit = int(math.Ceil(float64(totalVMs) / float64(MaxVMsPerWorkerNode)))
	}

	numBatches := max(batchesCPU, batchesMemory, minBatchesFromVMLimit)

	if numBatches < 1 {
		numBatches = 1
	}

	// Cap batches to totalVMs (prevent empty batches)
	if totalVMs > 0 && numBatches > totalVMs {
		numBatches = totalVMs
	}

	// Check if VM limit forces too many batches (larger nodes won't help)
	if numBatches > MaxBatches {
		return nil, fmt.Errorf("cluster has too many VMs (%d) to size within constraints (max %d VMs per node, max %d batches). "+
			"This inventory exceeds the sizing limits",
			totalVMs, MaxVMsPerWorkerNode, MaxBatches)
	}

	// Distribute resources evenly across batches
	cpuPerBatch := totalCPU / float64(numBatches)
	memoryPerBatch := totalMemory / float64(numBatches)

	// Enforce minimum batch size (preserve VM limit)
	if cpuPerBatch < MinBatchCPU || memoryPerBatch < MinBatchMemory {
		batchesFromMinCPU := int(math.Ceil(totalCPU / MinBatchCPU))
		batchesFromMinMemory := int(math.Ceil(totalMemory / MinBatchMemory))
		numBatches = max(batchesFromMinCPU, batchesFromMinMemory, minBatchesFromVMLimit)

		if numBatches < 1 {
			numBatches = 1
		}

		cpuPerBatch = totalCPU / float64(numBatches)
		memoryPerBatch = totalMemory / float64(numBatches)
	}

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

	// Calculate VMs per batch (distribute evenly)
	vmsPerBatch := totalVMs / numBatches
	remainingVMs := totalVMs % numBatches

	services := make([]BatchedService, numBatches)
	for i := 0; i < numBatches; i++ {
		// Distribute remaining VMs to first batches
		vmCount := vmsPerBatch
		if i < remainingVMs {
			vmCount++
		}

		services[i] = BatchedService{
			Name:           fmt.Sprintf("vms-batch-%d-services", i+1),
			RequiredCPU:    requiredCPU,
			RequiredMemory: requiredMemory,
			LimitCPU:       limitCPU,
			LimitMemory:    limitMemory,
			VMCount:        vmCount,
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

func effectiveControlPlaneNodeCount(req *mappers.ClusterRequirementsRequestForm) int {
	if req.HostedControlPlane {
		return 0
	}
	return req.ControlPlaneNodeCount
}

// BuildServiceAvoidLists enforces MaxVMsPerWorkerNode via avoid lists.
//
// Assumption: when limiting how many batches may share a node, we use
// maxVMsPerBatch = max(VMCount per batch)—i.e. we assume each batch on that node
// could be as large as the worst batch. Then maxServicesPerNode =
// MaxVMsPerWorkerNode/maxVMsPerBatch is a safe cap on batch count per node. That
// stays correct when VMCount differs across batches.
func BuildServiceAvoidLists(services []BatchedService) [][]string {
	numServices := len(services)
	if numServices == 0 {
		return [][]string{}
	}

	// Find max VMs in any batch
	maxVMsPerBatch := 0
	for _, svc := range services {
		if svc.VMCount > maxVMsPerBatch {
			maxVMsPerBatch = svc.VMCount
		}
	}

	if maxVMsPerBatch == 0 {
		// One avoid list per service; buildSizerPayload indexes by service index (not an empty slice).
		return make([][]string, numServices)
	}

	// Max services per node within VM limit
	maxServicesPerNode := MaxVMsPerWorkerNode / maxVMsPerBatch
	if maxServicesPerNode < 1 {
		maxServicesPerNode = 1
	}
	if maxServicesPerNode > numServices {
		maxServicesPerNode = numServices
	}

	// Different groups avoid each other
	serviceAvoids := make([][]string, numServices)
	for i := range numServices {
		currentGroup := i / maxServicesPerNode
		avoidList := []string{}

		for j := range numServices {
			if i == j {
				continue
			}
			otherGroup := j / maxServicesPerNode
			if otherGroup != currentGroup {
				avoidList = append(avoidList, services[j].Name)
			}
		}
		serviceAvoids[i] = avoidList
	}

	return serviceAvoids
}

// buildSizerPayload transforms batched services into sizer API format
// It uses avoid relationships to limit the number of services (and thus VMs) per node
func (s *SizerService) buildSizerPayload(
	services []BatchedService,
	platform string,
	effectiveWorkerNodeCPU float64,
	workerNodeMemory int,
	includeControlPlane bool,
	controlPlaneSchedulable bool,
	controlPlaneCPU int,
	controlPlaneMemory int,
	controlPlaneNodeCount int,
	singleNode bool,
) *client.SizerRequest {
	if singleNode {
		return s.buildSingleNodeSizerPayload(
			services,
			platform,
			controlPlaneCPU,
			controlPlaneMemory,
		)
	}

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
				Count:        controlPlaneNodeCount,
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

	// Build avoid relationships to enforce VM limit per node
	serviceAvoids := BuildServiceAvoidLists(services)
	if len(services) == 0 {
		return &client.SizerRequest{
			Platform:    platform,
			MachineSets: machineSets,
			Workloads:   workloads,
			Detailed:    true,
		}
	}

	// Build service descriptors with avoid relationships
	for i, svc := range services {
		workloads[vmWorkloadIndex].Services[i] = client.ServiceDescriptor{
			Name:           svc.Name,
			RequiredCPU:    svc.RequiredCPU,
			RequiredMemory: svc.RequiredMemory,
			LimitCPU:       svc.LimitCPU,
			LimitMemory:    svc.LimitMemory,
			Zones:          1,
			RunsWith:       []string{},
			Avoid:          serviceAvoids[i],
		}
	}

	return &client.SizerRequest{
		Platform:    platform,
		MachineSets: machineSets,
		Workloads:   workloads,
		Detailed:    true,
	}
}

func (s *SizerService) buildSingleNodeSizerPayload(
	services []BatchedService,
	platform string,
	controlPlaneCPU int,
	controlPlaneMemory int,
) *client.SizerRequest {
	machineSets := []client.MachineSet{
		{
			Name:                    "controlPlane",
			CPU:                     controlPlaneCPU,
			Memory:                  controlPlaneMemory,
			InstanceName:            "control-plane",
			NumberOfDisks:           MachineSetNumberOfDisks,
			OnlyFor:                 []string{},
			Label:                   "Control Plane",
			AllowWorkloadScheduling: util.BoolPtr(true),
			ControlPlaneReserved: &client.ControlPlaneReserved{
				CPU:    ControlPlaneReservedCPU,
				Memory: ControlPlaneReservedMemory,
			},
		},
	}

	allServiceNames := make([]string, len(services))
	for i := range services {
		allServiceNames[i] = services[i].Name
	}

	vmServices := make([]client.ServiceDescriptor, len(services))
	for i, svc := range services {
		runsWith := make([]string, 0, len(services)-1)
		for j := range services {
			if j != i {
				runsWith = append(runsWith, allServiceNames[j])
			}
		}
		vmServices[i] = client.ServiceDescriptor{
			Name:           svc.Name,
			RequiredCPU:    svc.RequiredCPU,
			RequiredMemory: svc.RequiredMemory,
			LimitCPU:       svc.LimitCPU,
			LimitMemory:    svc.LimitMemory,
			Zones:          1,
			RunsWith:       runsWith,
			Avoid:          []string{},
		}
	}

	workloads := []client.Workload{
		{
			Name:         "control-plane-services",
			Count:        1,
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
		{
			Name:         "vm-workload",
			Count:        1,
			UsesMachines: []string{"controlPlane"},
			Services:     vmServices,
		},
	}

	return &client.SizerRequest{
		Platform:    platform,
		MachineSets: machineSets,
		Workloads:   workloads,
		Detailed:    true,
	}
}

// transformSizerResponse maps sizer service response to API response.
func (s *SizerService) transformSizerResponse(sizerResponse *client.SizerResponse, controlPlaneNodeCount int) TransformedSizerResponse {
	workerNodes := 0
	controlPlaneNodes := 0

	if len(sizerResponse.Data.Advanced) > 0 {
		// Count nodes from Advanced field
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
		// Fallback when Advanced is missing (legacy/error cases). Normally sizer returns Advanced.
		total := sizerResponse.Data.NodeCount
		if total < controlPlaneNodeCount {
			controlPlaneNodes = total
			workerNodes = 0
		} else {
			controlPlaneNodes = controlPlaneNodeCount
			workerNodes = total - controlPlaneNodeCount
		}
	}

	failoverNodes := calculateFailoverNodes(workerNodes)
	workerNodes += failoverNodes

	totalNodes := controlPlaneNodes + workerNodes

	if controlPlaneNodeCount == 1 {
		totalNodes = 1
		controlPlaneNodes = 1
		workerNodes = 0
		failoverNodes = 0
	}

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

// isSizerSchedulabilityError detects schedulability failures by sizer error message substrings.
func isSizerSchedulabilityError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "not schedulable") ||
		strings.Contains(msg, "Minimum required") ||
		strings.Contains(msg, "too small")
}

func (s *SizerService) singleNodeFitError(totalCPU, totalMemory int, smtMultiplier float64, controlPlaneCPU, controlPlaneMemory int) error {
	// Calculate uncapped minimum to detect if workload truly exceeds max supported size
	denominator := 1.0 * CapacityMultiplier
	minEffectiveCPUPerNode := float64(totalCPU) / denominator
	uncappedMinNodeCPU := int(math.Ceil(minEffectiveCPUPerNode / smtMultiplier))
	uncappedMinNodeMemory := int(math.Ceil(float64(totalMemory) / denominator))

	// Round up to nearest even number for CPU, nearest multiple of 4 for memory
	uncappedMinNodeCPU = int(math.Ceil(float64(uncappedMinNodeCPU)/2) * 2)
	uncappedMinNodeMemory = int(math.Ceil(float64(uncappedMinNodeMemory)/4) * 4)

	// If workload truly exceeds max supported size, recommend multi-node only
	if uncappedMinNodeCPU > MaxRecommendedNodeCPU || uncappedMinNodeMemory > MaxRecommendedNodeMemory {
		return NewErrInvalidRequest("workload does not fit on a single node. Use a multi-node cluster.")
	}

	// Otherwise use the calculated minimum (already below max)
	// Ensure minimum is at least control plane reserve so the suggested size can schedule the CP
	minReservedCPU := int(math.Ceil(ControlPlaneReservedCPU/2.0) * 2)
	minReservedMemory := int(math.Ceil(ControlPlaneReservedMemory/4.0) * 4)
	minNodeCPU := max(max(uncappedMinNodeCPU, MinFallbackNodeCPU), minReservedCPU)
	minNodeMemory := max(max(uncappedMinNodeMemory, MinFallbackNodeMemory), minReservedMemory)

	return NewErrInvalidRequest(singleNodeFitErrorMessage(controlPlaneCPU, controlPlaneMemory, minNodeCPU, minNodeMemory))
}

// singleNodeFitErrorMessage returns the error when single-node didn't fit. If the user
// is already at/above our minimum or at max, we recommend multi-node only.
func singleNodeFitErrorMessage(controlPlaneCPU, controlPlaneMemory, minNodeCPU, minNodeMemory int) string {
	alreadyAtOrAbove := controlPlaneCPU >= minNodeCPU && controlPlaneMemory >= minNodeMemory
	atMaxSupported := controlPlaneCPU >= MaxRecommendedNodeCPU && controlPlaneMemory >= MaxRecommendedNodeMemory
	if alreadyAtOrAbove || atMaxSupported {
		return "workload does not fit on a single node. Use a multi-node cluster."
	}
	return fmt.Sprintf(
		"workload does not fit on a single node with the specified resources. Use at least %d CPU / %d GB memory per node for a single-node cluster, or use a multi-node cluster",
		minNodeCPU, minNodeMemory,
	)
}
