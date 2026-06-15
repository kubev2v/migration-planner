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

	inventoryapi "github.com/kubev2v/migration-planner-common/api/inventory"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/client"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
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

	// Control plane defaults; must match api/v1alpha1/openapi.yaml.
	defaultControlPlaneCPU         = 6
	defaultControlPlaneMemory      = 16
	defaultControlPlaneNodeCount   = 3
	defaultControlPlaneSchedulable = false
	defaultHostedControlPlane      = false
	defaultWorkerNodeThreads       = 0

	MinConfidenceThreshold = 50.0
)

type SizerServicer interface {
	CalculateClusterRequirements(ctx context.Context, assessmentID uuid.UUID, req *mappers.ClusterRequirementsRequestForm) (*api.ClusterRequirementsResponse, error)
	CalculateStandaloneClusterRequirements(ctx context.Context, req *mappers.StandaloneClusterRequirementsRequestForm) (*mappers.StandaloneClusterRequirementsResponseForm, error)
	GetClusterRequirementsInput(ctx context.Context, assessmentID uuid.UUID, clusterID string) (*mappers.ClusterRequirementsInputForm, error)
	Health(ctx context.Context) error
}

type UtilizationContext struct {
	CpuMultiplier    float64
	MemoryMultiplier float64
	CpuPercent       float64
	MemoryPercent    float64
	Confidence       float64
	HasData          bool
}

type SizingResult struct {
	TotalNodes          int
	WorkerNodes         int
	TotalCPU            int
	TotalMemory         int
	EffectiveCPU        float64
	EffectiveMemory     float64
	Services            []BatchedService
	ResourceConsumption *client.ResourceConsumption
}

// ToMapperSizingResult converts service.SizingResult to mappers.SizingResult
func (s SizingResult) ToMapperSizingResult() mappers.SizingResult {
	return mappers.SizingResult{
		TotalNodes:          s.TotalNodes,
		WorkerNodes:         s.WorkerNodes,
		TotalCPU:            s.TotalCPU,
		TotalMemory:         s.TotalMemory,
		EffectiveCPU:        s.EffectiveCPU,
		EffectiveMemory:     s.EffectiveMemory,
		ResourceConsumption: convertResourceConsumption(s.ResourceConsumption),
	}
}

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

type clusterRequirementsParams struct {
	TotalVMs                int
	TotalCPU                int
	TotalMemory             int
	WorkerNodeCPU           int
	WorkerNodeMemory        int
	WorkerNodeThreads       *int
	CpuOverCommitRatio      string
	MemoryOverCommitRatio   string
	ControlPlaneSchedulable *bool
	ControlPlaneCPU         *int
	ControlPlaneMemory      *int
	ControlPlaneNodeCount   int
	HostedControlPlane      bool
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

func (s *SizerService) extractUtilizationFromInventory(
	inventory *inventoryapi.Inventory,
	clusterID string,
) UtilizationContext {
	if inventory == nil {
		return UtilizationContext{
			CpuMultiplier:    1.0,
			MemoryMultiplier: 1.0,
			CpuPercent:       100.0,
			MemoryPercent:    100.0,
			Confidence:       0.0,
			HasData:          false,
		}
	}

	clusterData, exists := inventory.Clusters[clusterID]
	if !exists {
		return UtilizationContext{
			CpuMultiplier:    1.0,
			MemoryMultiplier: 1.0,
			CpuPercent:       100.0,
			MemoryPercent:    100.0,
			Confidence:       0.0,
			HasData:          false,
		}
	}

	utilization := clusterData.ClusterUtilization
	if utilization == nil {
		return UtilizationContext{
			CpuMultiplier:    1.0,
			MemoryMultiplier: 1.0,
			CpuPercent:       100.0,
			MemoryPercent:    100.0,
			Confidence:       0.0,
			HasData:          false,
		}
	}

	if utilization.Confidence < MinConfidenceThreshold {
		return UtilizationContext{
			CpuMultiplier:    1.0,
			MemoryMultiplier: 1.0,
			CpuPercent:       100.0,
			MemoryPercent:    100.0,
			Confidence:       utilization.Confidence,
			HasData:          false,
		}
	}

	cpuPct := utilization.CpuMax
	memPct := utilization.MemMax

	if math.IsNaN(cpuPct) || math.IsInf(cpuPct, 0) || cpuPct <= 0 ||
		math.IsNaN(memPct) || math.IsInf(memPct, 0) || memPct <= 0 {
		return UtilizationContext{
			CpuMultiplier:    1.0,
			MemoryMultiplier: 1.0,
			CpuPercent:       100.0,
			MemoryPercent:    100.0,
			Confidence:       utilization.Confidence,
			HasData:          false,
		}
	}

	cpuMultiplier := math.Min(cpuPct/100.0, 1.0)
	memMultiplier := math.Min(memPct/100.0, 1.0)

	effectiveCpuPct := cpuMultiplier * 100.0
	effectiveMemPct := memMultiplier * 100.0

	return UtilizationContext{
		CpuMultiplier:    cpuMultiplier,
		MemoryMultiplier: memMultiplier,
		CpuPercent:       effectiveCpuPct,
		MemoryPercent:    effectiveMemPct,
		Confidence:       utilization.Confidence,
		HasData:          true,
	}
}

func extractWorkerNodeThreads(params *clusterRequirementsParams) int {
	if params.WorkerNodeThreads != nil {
		return *params.WorkerNodeThreads
	}
	return 0
}

func extractControlPlaneSchedulable(params *clusterRequirementsParams) bool {
	if params.ControlPlaneSchedulable != nil {
		return *params.ControlPlaneSchedulable
	}
	return false
}

func extractControlPlaneCPU(params *clusterRequirementsParams) int {
	if params.ControlPlaneCPU != nil {
		return *params.ControlPlaneCPU
	}
	return 0
}

func extractControlPlaneMemory(params *clusterRequirementsParams) int {
	if params.ControlPlaneMemory != nil {
		return *params.ControlPlaneMemory
	}
	return 0
}

func (s *SizerService) calculateSizingWithMultipliers(
	ctx context.Context,
	totalCPU int,
	totalMemory int,
	totalVMs int,
	params clusterRequirementsParams,
	cpuMultiplier float64,
	memoryMultiplier float64,
) (SizingResult, error) {
	effectiveTotalCPU := float64(totalCPU) * cpuMultiplier
	effectiveTotalMemory := float64(totalMemory) * memoryMultiplier

	workerNodeThreads := extractWorkerNodeThreads(&params)
	effectiveCPU := CalculateEffectiveCPU(params.WorkerNodeCPU, workerNodeThreads)

	services, err := s.aggregateVMsIntoServices(
		effectiveTotalCPU,
		effectiveTotalMemory,
		totalVMs,
		effectiveCPU,
		params.WorkerNodeMemory,
		params.CpuOverCommitRatio,
		params.MemoryOverCommitRatio,
		CapacityMultiplier,
	)
	if err != nil {
		return SizingResult{}, fmt.Errorf("aggregating services: %w", err)
	}

	controlPlaneSchedulable := extractControlPlaneSchedulable(&params)
	controlPlaneCPU := extractControlPlaneCPU(&params)
	controlPlaneMemory := extractControlPlaneMemory(&params)

	includeControlPlane := !params.HostedControlPlane
	singleNode := params.ControlPlaneNodeCount == 1

	sizerPayload := s.buildSizerPayload(
		services,
		DefaultPlatform,
		effectiveCPU,
		params.WorkerNodeMemory,
		includeControlPlane,
		controlPlaneSchedulable,
		controlPlaneCPU,
		controlPlaneMemory,
		params.ControlPlaneNodeCount,
		singleNode,
	)

	sizerResponse, err := s.sizerClient.CalculateSizing(ctx, sizerPayload)
	if err != nil {
		if singleNode && isSizerSchedulabilityError(err) {
			smtMultiplier := 1.0
			if params.WorkerNodeCPU > 0 {
				smtMultiplier = effectiveCPU / float64(params.WorkerNodeCPU)
			}
			controlPlaneCPU := 0
			if params.ControlPlaneCPU != nil {
				controlPlaneCPU = *params.ControlPlaneCPU
			}
			controlPlaneMemory := 0
			if params.ControlPlaneMemory != nil {
				controlPlaneMemory = *params.ControlPlaneMemory
			}
			return SizingResult{}, s.singleNodeFitError(totalCPU, totalMemory, smtMultiplier, params.CpuOverCommitRatio, params.MemoryOverCommitRatio, controlPlaneCPU, controlPlaneMemory)
		}
		return SizingResult{}, fmt.Errorf("failed to call sizer service: %w", err)
	}
	if sizerResponse == nil {
		return SizingResult{}, fmt.Errorf("sizer service returned empty response")
	}

	_, err = s.validateVMDistribution(sizerResponse, services, singleNode)
	if err != nil {
		return SizingResult{}, err
	}

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
		total := sizerResponse.Data.NodeCount
		if total < params.ControlPlaneNodeCount {
			controlPlaneNodes = total
			workerNodes = 0
		} else {
			controlPlaneNodes = params.ControlPlaneNodeCount
			workerNodes = total - params.ControlPlaneNodeCount
		}
	}

	totalNodes := controlPlaneNodes + workerNodes

	singleNode = params.ControlPlaneNodeCount == 1
	if singleNode && totalNodes <= 1 {
		totalNodes = 1
		workerNodes = 0
	}

	return SizingResult{
		TotalNodes:          totalNodes,
		WorkerNodes:         workerNodes,
		TotalCPU:            sizerResponse.Data.TotalCPU,
		TotalMemory:         sizerResponse.Data.TotalMemory,
		EffectiveCPU:        effectiveTotalCPU,
		EffectiveMemory:     effectiveTotalMemory,
		Services:            services,
		ResourceConsumption: &sizerResponse.Data.ResourceConsumption,
	}, nil
}

// validateVMDistribution checks that no worker node exceeds MaxVMsPerWorkerNode.
// Returns the maximum VMs found on any node, or error if constraint violated.
func (s *SizerService) validateVMDistribution(
	sizerResponse *client.SizerResponse,
	services []BatchedService,
	singleNode bool,
) (int, error) {
	serviceToVMCount := make(map[string]int)
	for _, svc := range services {
		serviceToVMCount[svc.Name] = svc.VMCount
	}

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
		return maxVMsPerNode, NewErrInvalidRequest(
			fmt.Sprintf("VM distribution constraint violated: found %d VMs on a node, exceeds limit of %d per node",
				maxVMsPerNode, MaxVMsPerWorkerNode))
	}

	return maxVMsPerNode, nil
}

// CalculateClusterRequirements calculates cluster requirements for an assessment
func (s *SizerService) CalculateClusterRequirements(
	ctx context.Context,
	assessmentID uuid.UUID,
	req *mappers.ClusterRequirementsRequestForm,
) (*api.ClusterRequirementsResponse, error) {
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

	calcReq := applyDefaults(req)

	if len(assessment.Snapshots) == 0 {
		return nil, fmt.Errorf("assessment has no snapshots")
	}

	latestSnapshot := assessment.Snapshots[0]
	if len(latestSnapshot.Inventory) == 0 {
		return nil, fmt.Errorf("latest snapshot has empty inventory")
	}

	var inventory inventoryapi.Inventory
	if err := json.Unmarshal(latestSnapshot.Inventory, &inventory); err != nil {
		return nil, fmt.Errorf("failed to parse inventory: %w", err)
	}

	if len(inventory.Clusters) == 0 {
		return nil, fmt.Errorf("inventory has no clusters")
	}

	clusterInventory, exists := inventory.Clusters[calcReq.ClusterID]
	if !exists {
		return nil, fmt.Errorf("cluster %s not found in assessment %s", calcReq.ClusterID, assessmentID)
	}

	totalVMs := clusterInventory.Vms.Total
	totalCPU := clusterInventory.Vms.CpuCores.Total
	totalMemory := clusterInventory.Vms.RamGB.Total

	if totalVMs == 0 || totalCPU == 0 || totalMemory == 0 {
		return nil, NewErrInvalidClusterInventory(calcReq.ClusterID, "cluster has no VMs or no CPU/Memory resources and cannot be used for migration planning")
	}

	params := clusterRequirementsParams{
		TotalVMs:                totalVMs,
		TotalCPU:                totalCPU,
		TotalMemory:             totalMemory,
		WorkerNodeCPU:           calcReq.WorkerNodeCPU,
		WorkerNodeMemory:        calcReq.WorkerNodeMemory,
		WorkerNodeThreads:       calcReq.WorkerNodeThreads,
		CpuOverCommitRatio:      calcReq.CpuOverCommitRatio,
		MemoryOverCommitRatio:   calcReq.MemoryOverCommitRatio,
		ControlPlaneSchedulable: calcReq.ControlPlaneSchedulable,
		ControlPlaneCPU:         calcReq.ControlPlaneCPU,
		ControlPlaneMemory:      calcReq.ControlPlaneMemory,
		ControlPlaneNodeCount:   effectiveControlPlaneNodeCount(calcReq),
		HostedControlPlane:      calcReq.HostedControlPlane != nil && *calcReq.HostedControlPlane,
	}

	singleNode := params.ControlPlaneNodeCount == 1
	controlPlaneSchedulable := extractControlPlaneSchedulable(&params)
	workerNodeThreads := extractWorkerNodeThreads(&params)
	effectiveCPU := CalculateEffectiveCPU(params.WorkerNodeCPU, workerNodeThreads)
	smtMultiplier := 1.0
	if params.WorkerNodeCPU > 0 && workerNodeThreads > 0 && workerNodeThreads > params.WorkerNodeCPU {
		smtMultiplier = effectiveCPU / float64(params.WorkerNodeCPU)
	}

	if singleNode && !controlPlaneSchedulable {
		return nil, NewErrInvalidRequest(
			"single-node clusters require schedulable control planes. " +
				"Set ControlPlaneSchedulable to true or use multiple control plane nodes")
	}

	if !singleNode {
		targetCPU := effectiveCPU * CapacityMultiplier
		targetMemory := float64(params.WorkerNodeMemory) * CapacityMultiplier

		minNodeCPUForMaxBatches, minNodeMemoryForMaxBatches := s.calculateMinimumNodeSize(
			params.TotalCPU,
			params.TotalMemory,
			MaxBatches,
			CapacityMultiplier,
			smtMultiplier,
		)

		estimatedBatchesCPU := int(math.Ceil(float64(params.TotalCPU) / targetCPU))
		estimatedBatchesMemory := int(math.Ceil(float64(params.TotalMemory) / targetMemory))
		estimatedBatches := max(estimatedBatchesCPU, estimatedBatchesMemory)

		if estimatedBatches > MaxBatches {
			return nil, s.formatNodeSizeError(
				params.WorkerNodeCPU, params.WorkerNodeMemory,
				params.TotalCPU, params.TotalMemory,
				minNodeCPUForMaxBatches, minNodeMemoryForMaxBatches,
			)
		}
	}

	utilizationContext := s.extractUtilizationFromInventory(&inventory, calcReq.ClusterID)

	// Track optimization attempt status
	optimizationStatus := api.OptimizationStatus{
		Attempted: false,
		Reason:    api.NoUtilizationData,
	}

	baselineResult, err := s.calculateSizingWithMultipliers(
		ctx,
		totalCPU,
		totalMemory,
		totalVMs,
		params,
		1.0,
		1.0,
	)
	if err != nil {
		var invalidReq *ErrInvalidRequest
		if errors.As(err, &invalidReq) {
			return nil, err
		}
		return nil, fmt.Errorf("calculating baseline sizing: %w", err)
	}

	if !singleNode && baselineResult.TotalNodes > MaxNodeCount {
		minNodeCPU, minNodeMemory := s.calculateMinimumNodeSize(
			params.TotalCPU,
			params.TotalMemory,
			MaxNodeCount,
			CapacityMultiplier,
			smtMultiplier,
		)
		return nil, s.formatNodeSizeError(
			params.WorkerNodeCPU, params.WorkerNodeMemory,
			params.TotalCPU, params.TotalMemory,
			minNodeCPU, minNodeMemory,
		)
	}

	if singleNode && baselineResult.TotalNodes > 1 {
		controlPlaneCPU := extractControlPlaneCPU(&params)
		controlPlaneMemory := extractControlPlaneMemory(&params)
		return nil, s.singleNodeFitError(totalCPU, totalMemory, smtMultiplier, params.CpuOverCommitRatio, params.MemoryOverCommitRatio, controlPlaneCPU, controlPlaneMemory)
	}

	logger.Operation("calculate_baseline").
		WithString("cluster_id", calcReq.ClusterID).
		WithInt("total_nodes", baselineResult.TotalNodes).
		WithInt("worker_nodes", baselineResult.WorkerNodes).
		Build()

	var optimizedResult *SizingResult
	if !utilizationContext.HasData {
		// Check why optimization is not being attempted
		if utilizationContext.Confidence > 0 && utilizationContext.Confidence < MinConfidenceThreshold {
			optimizationStatus.Reason = api.LowConfidence
		}
		// optimizationStatus.Attempted remains false
	} else {
		// Utilization data available - attempt optimization
		optimizationStatus.Attempted = true
		result, err := s.calculateSizingWithMultipliers(
			ctx,
			totalCPU,
			totalMemory,
			totalVMs,
			params,
			utilizationContext.CpuMultiplier,
			utilizationContext.MemoryMultiplier,
		)
		if err != nil {
			optimizationStatus.Reason = api.CalculationError
			logger.Operation("calculate_optimized_error").
				WithString("cluster_id", calcReq.ClusterID).
				WithParam("error", err).
				Build().
				Error(err).
				Log()
		} else {
			optimizationStatus.Reason = api.Success
			optimizedResult = &result
			logger.Operation("calculate_optimized").
				WithString("cluster_id", calcReq.ClusterID).
				WithInt("total_nodes", result.TotalNodes).
				WithInt("worker_nodes", result.WorkerNodes).
				WithParam("cpu_utilization_max", utilizationContext.CpuPercent).
				WithParam("memory_utilization_max", utilizationContext.MemoryPercent).
				WithParam("confidence", utilizationContext.Confidence).
				Build().
				Success().
				Log()
		}
	}

	if err := s.persistClusterSizingInput(ctx, assessmentID, req); err != nil {
		logger.Operation("persist_cluster_sizing_input").Build().Error(err).Log()
		return nil, err
	}

	baselineFailoverNodes := calculateFailoverNodes(baselineResult.WorkerNodes)

	var optimizedFailoverNodes int
	if optimizedResult != nil {
		optimizedFailoverNodes = calculateFailoverNodes(optimizedResult.WorkerNodes)
	}

	mapper := &mappers.Mapper{}
	baselineForMapper := baselineResult.ToMapperSizingResult()

	var optimizedForMapper *mappers.SizingResult
	if optimizedResult != nil {
		mapped := optimizedResult.ToMapperSizingResult()
		optimizedForMapper = &mapped
	}

	utilizationForMapper := mappers.UtilizationContext{
		CpuMultiplier:    utilizationContext.CpuMultiplier,
		MemoryMultiplier: utilizationContext.MemoryMultiplier,
		CpuPercent:       utilizationContext.CpuPercent,
		MemoryPercent:    utilizationContext.MemoryPercent,
		Confidence:       utilizationContext.Confidence,
		HasData:          utilizationContext.HasData,
	}

	return mapper.ToClusterRequirementsResponse(
		baselineForMapper,
		optimizedForMapper,
		utilizationForMapper,
		baselineFailoverNodes,
		optimizedFailoverNodes,
		totalVMs,
		totalCPU,
		totalMemory,
		optimizationStatus,
	), nil
}

// CalculateStandaloneClusterRequirements calculates cluster sizing for inline inventory.
func (s *SizerService) CalculateStandaloneClusterRequirements(
	ctx context.Context,
	req *mappers.StandaloneClusterRequirementsRequestForm,
) (*mappers.StandaloneClusterRequirementsResponseForm, error) {
	if s.sizerClient == nil {
		return nil, fmt.Errorf("sizer client is not configured")
	}

	calcReq := applyStandaloneDefaults(req)

	params := clusterRequirementsParams{
		TotalVMs:                calcReq.TotalVMs,
		TotalCPU:                calcReq.TotalCPU,
		TotalMemory:             calcReq.TotalMemory,
		WorkerNodeCPU:           calcReq.WorkerNodeCPU,
		WorkerNodeMemory:        calcReq.WorkerNodeMemory,
		WorkerNodeThreads:       calcReq.WorkerNodeThreads,
		CpuOverCommitRatio:      calcReq.CpuOverCommitRatio,
		MemoryOverCommitRatio:   calcReq.MemoryOverCommitRatio,
		ControlPlaneSchedulable: calcReq.ControlPlaneSchedulable,
		ControlPlaneCPU:         calcReq.ControlPlaneCPU,
		ControlPlaneMemory:      calcReq.ControlPlaneMemory,
		ControlPlaneNodeCount:   effectiveStandaloneControlPlaneNodeCount(calcReq),
		HostedControlPlane:      calcReq.HostedControlPlane != nil && *calcReq.HostedControlPlane,
	}

	transformed, err := s.calculateClusterRequirementsInternal(ctx, params)
	if err != nil {
		return nil, err
	}

	return &mappers.StandaloneClusterRequirementsResponseForm{
		ClusterSizing:       transformed.ClusterSizing,
		ResourceConsumption: transformed.ResourceConsumption,
	}, nil
}

func (s *SizerService) calculateClusterRequirementsInternal(
	ctx context.Context,
	params clusterRequirementsParams,
) (TransformedSizerResponse, error) {
	logger := s.logger.WithContext(ctx)

	if params.TotalVMs <= 0 || params.TotalCPU <= 0 || params.TotalMemory <= 0 {
		return TransformedSizerResponse{}, NewErrInvalidRequest(
			"inventory must have positive VMs, CPU, and Memory values",
		)
	}

	includeControlPlane := !params.HostedControlPlane

	workerNodeThreads := extractWorkerNodeThreads(&params)
	controlPlaneSchedulable := extractControlPlaneSchedulable(&params)
	controlPlaneCPU := extractControlPlaneCPU(&params)
	controlPlaneMemory := extractControlPlaneMemory(&params)

	effectiveCPU := CalculateEffectiveCPU(params.WorkerNodeCPU, workerNodeThreads)
	if effectiveCPU <= 0 || params.WorkerNodeMemory <= 0 {
		return TransformedSizerResponse{}, NewErrInvalidRequest(
			fmt.Sprintf(
				"worker node size must be greater than zero: CPU=%.2f, Memory=%d",
				effectiveCPU,
				params.WorkerNodeMemory,
			),
		)
	}

	smtMultiplier := 1.0
	if params.WorkerNodeCPU > 0 && workerNodeThreads > 0 && workerNodeThreads > params.WorkerNodeCPU {
		smtMultiplier = effectiveCPU / float64(params.WorkerNodeCPU)
	}

	tracer := logger.Operation("calculate_cluster_requirements_internal").
		WithInt("total_vms", params.TotalVMs).
		WithInt("total_cpu", params.TotalCPU).
		WithInt("total_memory", params.TotalMemory).
		WithString("cpu_over_commit_ratio", params.CpuOverCommitRatio).
		WithString("memory_over_commit_ratio", params.MemoryOverCommitRatio).
		WithInt("worker_node_cpu", params.WorkerNodeCPU).
		WithInt("worker_node_threads", workerNodeThreads).
		WithString("worker_node_effective_cpu", fmt.Sprintf("%.2f", effectiveCPU)).
		WithInt("worker_node_memory", params.WorkerNodeMemory).
		WithBool("control_plane_schedulable", controlPlaneSchedulable).
		WithBool("hosted_control_plane", params.HostedControlPlane).
		WithInt("control_plane_node_count", params.ControlPlaneNodeCount).
		Build()

	singleNode := params.ControlPlaneNodeCount == 1

	if !singleNode {
		targetCPU := effectiveCPU * CapacityMultiplier
		targetMemory := float64(params.WorkerNodeMemory) * CapacityMultiplier

		minNodeCPUForMaxBatches, minNodeMemoryForMaxBatches := s.calculateMinimumNodeSize(
			params.TotalCPU,
			params.TotalMemory,
			MaxBatches,
			CapacityMultiplier,
			smtMultiplier,
		)

		estimatedBatchesCPU := int(math.Ceil(float64(params.TotalCPU) / targetCPU))
		estimatedBatchesMemory := int(math.Ceil(float64(params.TotalMemory) / targetMemory))
		estimatedBatches := max(estimatedBatchesCPU, estimatedBatchesMemory)

		if estimatedBatches > MaxBatches {
			return TransformedSizerResponse{}, s.formatNodeSizeError(
				params.WorkerNodeCPU, params.WorkerNodeMemory,
				params.TotalCPU, params.TotalMemory,
				minNodeCPUForMaxBatches, minNodeMemoryForMaxBatches,
			)
		}
	}

	services, err := s.aggregateVMsIntoServices(
		float64(params.TotalCPU),
		float64(params.TotalMemory),
		params.TotalVMs,
		effectiveCPU,
		params.WorkerNodeMemory,
		params.CpuOverCommitRatio,
		params.MemoryOverCommitRatio,
		CapacityMultiplier,
	)
	if err != nil {
		if singleNode {
			return TransformedSizerResponse{}, s.singleNodeFitError(params.TotalCPU, params.TotalMemory, smtMultiplier, params.CpuOverCommitRatio, params.MemoryOverCommitRatio, controlPlaneCPU, controlPlaneMemory)
		}
		return TransformedSizerResponse{}, NewErrInvalidRequest(err.Error())
	}

	tracer.Step("batching_complete").
		WithInt("num_services", len(services)).
		Log()

	if singleNode && !controlPlaneSchedulable {
		return TransformedSizerResponse{}, NewErrInvalidRequest(
			"single-node clusters require schedulable control planes. " +
				"Set ControlPlaneSchedulable to true or use multiple control plane nodes",
		)
	}

	sizerPayload := s.buildSizerPayload(
		services,
		DefaultPlatform,
		effectiveCPU,
		params.WorkerNodeMemory,
		includeControlPlane,
		controlPlaneSchedulable,
		controlPlaneCPU,
		controlPlaneMemory,
		params.ControlPlaneNodeCount,
		singleNode,
	)

	sizerResponse, err := s.sizerClient.CalculateSizing(ctx, sizerPayload)
	if err != nil {
		tracer.Error(err).Log()
		if singleNode && isSizerSchedulabilityError(err) {
			return TransformedSizerResponse{}, s.singleNodeFitError(params.TotalCPU, params.TotalMemory, smtMultiplier, params.CpuOverCommitRatio, params.MemoryOverCommitRatio, controlPlaneCPU, controlPlaneMemory)
		}
		return TransformedSizerResponse{}, fmt.Errorf("failed to call sizer service: %w", err)
	}
	if sizerResponse == nil {
		return TransformedSizerResponse{}, fmt.Errorf("sizer service returned empty response")
	}

	transformed := s.transformSizerResponse(sizerResponse, params.ControlPlaneNodeCount)

	if singleNode && sizerResponse.Data.NodeCount > 1 {
		return TransformedSizerResponse{}, s.singleNodeFitError(params.TotalCPU, params.TotalMemory, smtMultiplier, params.CpuOverCommitRatio, params.MemoryOverCommitRatio, controlPlaneCPU, controlPlaneMemory)
	}

	maxVMsPerNode, err := s.validateVMDistribution(sizerResponse, services, singleNode)
	if err != nil {
		if singleNode {
			return TransformedSizerResponse{}, s.singleNodeFitError(params.TotalCPU, params.TotalMemory, smtMultiplier, params.CpuOverCommitRatio, params.MemoryOverCommitRatio, controlPlaneCPU, controlPlaneMemory)
		}
		s.logger.Operation("vm_limit_exceeded").
			WithInt("max_vms_per_node", maxVMsPerNode).
			WithInt("limit", MaxVMsPerWorkerNode).
			Build().
			Error(err).
			Log()
		return TransformedSizerResponse{}, err
	}

	tracer.Step("sizer_results").
		WithInt("max_vms_per_node", maxVMsPerNode).
		WithInt("total_nodes", transformed.ClusterSizing.TotalNodes).
		WithInt("num_services", len(services)).
		Log()

	if transformed.ClusterSizing.TotalNodes > MaxNodeCount {
		minNodeCPU, minNodeMemory := s.calculateMinimumNodeSize(
			params.TotalCPU,
			params.TotalMemory,
			MaxNodeCount,
			CapacityMultiplier,
			smtMultiplier,
		)

		return TransformedSizerResponse{}, s.formatNodeSizeError(
			params.WorkerNodeCPU, params.WorkerNodeMemory,
			params.TotalCPU, params.TotalMemory,
			minNodeCPU, minNodeMemory,
		)
	}

	tracer.Success().
		WithInt("total_nodes", transformed.ClusterSizing.TotalNodes).
		WithInt("worker_nodes", transformed.ClusterSizing.WorkerNodes).
		WithInt("control_plane_nodes", transformed.ClusterSizing.ControlPlaneNodes).
		WithInt("failover_nodes", transformed.ClusterSizing.FailoverNodes).
		Log()

	return transformed, nil
}

func (s *SizerService) GetClusterRequirementsInput(
	ctx context.Context,
	assessmentID uuid.UUID,
	clusterID string,
) (*mappers.ClusterRequirementsInputForm, error) {
	input, err := s.store.ClusterSizingInput().Get(ctx, assessmentID, clusterID)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return nil, NewErrClusterRequirementsNotFound(clusterID, assessmentID)
		}
		return nil, fmt.Errorf("failed to get stored cluster requirements input: %w", err)
	}

	return &mappers.ClusterRequirementsInputForm{
		ClusterID:               input.ExternalClusterID,
		CpuOverCommitRatio:      input.CpuOverCommitRatio,
		MemoryOverCommitRatio:   input.MemoryOverCommitRatio,
		WorkerNodeCPU:           input.WorkerNodeCPU,
		WorkerNodeThreads:       input.WorkerNodeThreads,
		WorkerNodeMemory:        input.WorkerNodeMemory,
		ControlPlaneSchedulable: input.ControlPlaneSchedulable,
		ControlPlaneNodeCount:   input.ControlPlaneNodeCount,
		ControlPlaneCPU:         input.ControlPlaneCPU,
		ControlPlaneMemory:      input.ControlPlaneMemory,
		HostedControlPlane:      input.HostedControlPlane,
	}, nil
}

func (s *SizerService) persistClusterSizingInput(
	ctx context.Context,
	assessmentID uuid.UUID,
	req *mappers.ClusterRequirementsRequestForm,
) error {
	// Build input from ONLY what user provided (no defaults) - preserve sparse payload semantics
	input := model.AssessmentClusterSizingInput{
		AssessmentID:          assessmentID,
		ExternalClusterID:     req.ClusterID,
		CpuOverCommitRatio:    util.ToStrPtr(req.CpuOverCommitRatio),
		MemoryOverCommitRatio: util.ToStrPtr(req.MemoryOverCommitRatio),
		WorkerNodeCPU:         util.IntPtr(req.WorkerNodeCPU),
		WorkerNodeMemory:      util.IntPtr(req.WorkerNodeMemory),

		// Store optional fields exactly as provided (nil if omitted)
		WorkerNodeThreads:       req.WorkerNodeThreads,
		HostedControlPlane:      req.HostedControlPlane,
		ControlPlaneSchedulable: req.ControlPlaneSchedulable,
		ControlPlaneNodeCount:   req.ControlPlaneNodeCount,
		ControlPlaneCPU:         req.ControlPlaneCPU,
		ControlPlaneMemory:      req.ControlPlaneMemory,
	}

	_, err := s.store.ClusterSizingInput().Upsert(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to persist cluster requirements input: %w", err)
	}

	return nil
}

type clusterSizingOptions struct {
	WorkerNodeThreads       *int
	HostedControlPlane      *bool
	ControlPlaneSchedulable *bool
	ControlPlaneNodeCount   *int
	ControlPlaneCPU         *int
	ControlPlaneMemory      *int
}

func applyClusterSizingOptionDefaults(opts *clusterSizingOptions) {
	if opts.WorkerNodeThreads == nil {
		opts.WorkerNodeThreads = util.IntPtr(defaultWorkerNodeThreads)
	}
	if opts.HostedControlPlane == nil {
		opts.HostedControlPlane = util.BoolPtr(defaultHostedControlPlane)
	}
	hostedCP := opts.HostedControlPlane != nil && *opts.HostedControlPlane
	if !hostedCP {
		if opts.ControlPlaneSchedulable == nil {
			opts.ControlPlaneSchedulable = util.BoolPtr(defaultControlPlaneSchedulable)
		}
		if opts.ControlPlaneNodeCount == nil {
			opts.ControlPlaneNodeCount = util.IntPtr(defaultControlPlaneNodeCount)
		}
		if opts.ControlPlaneCPU == nil {
			opts.ControlPlaneCPU = util.IntPtr(defaultControlPlaneCPU)
		}
		if opts.ControlPlaneMemory == nil {
			opts.ControlPlaneMemory = util.IntPtr(defaultControlPlaneMemory)
		}
		return
	}
	opts.ControlPlaneSchedulable = nil
	opts.ControlPlaneNodeCount = nil
	opts.ControlPlaneCPU = nil
	opts.ControlPlaneMemory = nil
}

// applyDefaults creates a copy of the request with defaults applied for calculation purposes
func applyDefaults(req *mappers.ClusterRequirementsRequestForm) *mappers.ClusterRequirementsRequestForm {
	opts := clusterSizingOptions{
		WorkerNodeThreads:       req.WorkerNodeThreads,
		HostedControlPlane:      req.HostedControlPlane,
		ControlPlaneSchedulable: req.ControlPlaneSchedulable,
		ControlPlaneNodeCount:   req.ControlPlaneNodeCount,
		ControlPlaneCPU:         req.ControlPlaneCPU,
		ControlPlaneMemory:      req.ControlPlaneMemory,
	}
	applyClusterSizingOptionDefaults(&opts)

	return &mappers.ClusterRequirementsRequestForm{
		ClusterID:               req.ClusterID,
		CpuOverCommitRatio:      req.CpuOverCommitRatio,
		MemoryOverCommitRatio:   req.MemoryOverCommitRatio,
		WorkerNodeCPU:           req.WorkerNodeCPU,
		WorkerNodeMemory:        req.WorkerNodeMemory,
		WorkerNodeThreads:       opts.WorkerNodeThreads,
		HostedControlPlane:      opts.HostedControlPlane,
		ControlPlaneSchedulable: opts.ControlPlaneSchedulable,
		ControlPlaneNodeCount:   opts.ControlPlaneNodeCount,
		ControlPlaneCPU:         opts.ControlPlaneCPU,
		ControlPlaneMemory:      opts.ControlPlaneMemory,
	}
}

func applyStandaloneDefaults(req *mappers.StandaloneClusterRequirementsRequestForm) *mappers.StandaloneClusterRequirementsRequestForm {
	opts := clusterSizingOptions{
		WorkerNodeThreads:       req.WorkerNodeThreads,
		HostedControlPlane:      req.HostedControlPlane,
		ControlPlaneSchedulable: req.ControlPlaneSchedulable,
		ControlPlaneNodeCount:   req.ControlPlaneNodeCount,
		ControlPlaneCPU:         req.ControlPlaneCPU,
		ControlPlaneMemory:      req.ControlPlaneMemory,
	}
	applyClusterSizingOptionDefaults(&opts)

	return &mappers.StandaloneClusterRequirementsRequestForm{
		TotalVMs:                req.TotalVMs,
		TotalCPU:                req.TotalCPU,
		TotalMemory:             req.TotalMemory,
		CpuOverCommitRatio:      req.CpuOverCommitRatio,
		MemoryOverCommitRatio:   req.MemoryOverCommitRatio,
		WorkerNodeCPU:           req.WorkerNodeCPU,
		WorkerNodeMemory:        req.WorkerNodeMemory,
		WorkerNodeThreads:       opts.WorkerNodeThreads,
		HostedControlPlane:      opts.HostedControlPlane,
		ControlPlaneSchedulable: opts.ControlPlaneSchedulable,
		ControlPlaneNodeCount:   opts.ControlPlaneNodeCount,
		ControlPlaneCPU:         opts.ControlPlaneCPU,
		ControlPlaneMemory:      opts.ControlPlaneMemory,
	}
}

func effectiveStandaloneControlPlaneNodeCount(req *mappers.StandaloneClusterRequirementsRequestForm) int {
	if req.HostedControlPlane != nil && *req.HostedControlPlane {
		return 0
	}
	if req.ControlPlaneNodeCount != nil {
		return *req.ControlPlaneNodeCount
	}
	return 0
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
	if req.HostedControlPlane != nil && *req.HostedControlPlane {
		return 0
	}
	if req.ControlPlaneNodeCount != nil {
		return *req.ControlPlaneNodeCount
	}
	return 0
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

func convertResourceConsumption(rc *client.ResourceConsumption) *mappers.ResourceConsumption {
	if rc == nil {
		return nil
	}

	result := &mappers.ResourceConsumption{
		CPU:    rc.CPU,
		Memory: rc.Memory,
	}

	if rc.Limits != nil {
		result.Limits = &mappers.ResourceLimits{
			CPU:    rc.Limits.CPU,
			Memory: rc.Limits.Memory,
		}
	}

	if rc.OverCommitRatio != nil {
		result.OverCommitRatio = &mappers.OverCommitRatio{
			CPU:    rc.OverCommitRatio.CPU,
			Memory: rc.OverCommitRatio.Memory,
		}
	}

	return result
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

func (s *SizerService) singleNodeFitError(totalCPU, totalMemory int, smtMultiplier float64, cpuOverCommitRatio, memoryOverCommitRatio string, controlPlaneCPU, controlPlaneMemory int) error {
	// Apply over-commit ratios to get actual resource requests
	cpuOverCommitMultiplier, err := s.getCpuOverCommitMultiplier(cpuOverCommitRatio)
	if err != nil {
		return err
	}
	memoryOverCommitMultiplier, err := s.getMemoryOverCommitMultiplier(memoryOverCommitRatio)
	if err != nil {
		return err
	}

	actualCPU := float64(totalCPU) / cpuOverCommitMultiplier
	actualMemory := float64(totalMemory) / memoryOverCommitMultiplier

	denominator := 1.0 * CapacityMultiplier
	minEffectiveCPUPerNode := actualCPU / denominator
	uncappedMinNodeCPU := int(math.Ceil(minEffectiveCPUPerNode / smtMultiplier))
	uncappedMinNodeMemory := int(math.Ceil(actualMemory / denominator))

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

	// Ensure recommendations are at least as large as current values
	recommendedCPU := max(controlPlaneCPU, minNodeCPU)
	recommendedMemory := max(controlPlaneMemory, minNodeMemory)

	return fmt.Sprintf(
		"workload does not fit on a single node with the specified resources. Use at least %d CPU / %d GB memory per node for a single-node cluster, or use a multi-node cluster",
		recommendedCPU, recommendedMemory,
	)
}
