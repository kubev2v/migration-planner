package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner-common/estimations/complexity"
	"github.com/kubev2v/migration-planner-common/estimations/engines"
	"github.com/kubev2v/migration-planner-common/estimations/estimation"
	"github.com/kubev2v/migration-planner-common/estimations/estimation/calculators"
	svcmodel "github.com/kubev2v/migration-planner/internal/model"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/pkg/log"
)

type EstimationServicer interface {
	CalculateMigrationEstimation(ctx context.Context, assessmentID uuid.UUID, clusterIDs []string, schemas []engines.Schema, userParams []estimation.Param) (*svcmodel.MigrationEstimationResult, error)
	CalculateMigrationComplexity(ctx context.Context, assessmentID uuid.UUID, clusterIDs []string) (*svcmodel.MigrationComplexityResult, error)
	CalculateMigrationEstimationByComplexity(ctx context.Context, assessmentID uuid.UUID, clusterIDs []string, schemas []engines.Schema, userParams []estimation.Param) (*svcmodel.MigrationEstimationByComplexityResult, error)
}

// EstimationService orchestrates the migration time estimation workflow.
// It retrieves assessment and inventory data from the store and runs them
// through the estimation Engine to produce a MigrationAssessmentResult.
type EstimationService struct {
	store  store.Store
	logger *log.StructuredLogger
}

// NewEstimationService creates an EstimationService.
func NewEstimationService(store store.Store) *EstimationService {
	return &EstimationService{
		store:  store,
		logger: log.NewDebugLogger("estimation_service"),
	}
}

// CalculateMigrationEstimation calculates migration time estimation for selected clusters.
// When clusterIDs is empty, the vCenter-level aggregate inventory is used.
func (es *EstimationService) CalculateMigrationEstimation(
	ctx context.Context,
	assessmentID uuid.UUID,
	clusterIDs []string,
	schemas []engines.Schema,
	userParams []estimation.Param,
) (*svcmodel.MigrationEstimationResult, error) {
	logger := es.logger.WithContext(ctx)
	tracer := logger.Operation("calculate_migration_estimation").
		WithUUID("assessment_id", assessmentID).
		Build()

	err := engines.ValidateParams(userParams)
	if err != nil {
		return nil, &ErrInvalidEstimationParam{Msg: err.Error()}
	}

	inventory, err := loadAssessmentInventory(ctx, es.store, assessmentID)
	if err != nil {
		tracer.Error(err).Log()
		return nil, err
	}

	invData, err := resolveEstimationInventoryData(inventory, clusterIDs, assessmentID)
	if err != nil {
		tracer.Error(err).Log()
		return nil, err
	}

	baseParams := engines.BuildBaseParams(userParams...)
	params := append(baseParams,
		estimation.Param{Key: calculators.ParamVMCount, Value: invData.Vms.Total},
		estimation.Param{Key: calculators.ParamTotalDiskGB, Value: float64(invData.Vms.DiskGB.Total)},
	)

	tracer.Step("mapped_params").WithInt("param_count", len(params)).Log()

	results, err := runEstimation(schemas, params)
	if err != nil {
		return nil, err
	}

	tracer.Success().
		WithInt("schema_count", len(results)).
		Log()

	resolvedSchemas := make([]engines.Schema, 0, len(results))
	for schema := range results {
		resolvedSchemas = append(resolvedSchemas, schema)
	}

	return &svcmodel.MigrationEstimationResult{
		Estimation:        results,
		EstimationContext: svcmodel.EstimationContext{Schemas: resolvedSchemas, BaseParams: baseParams},
	}, nil
}

// CalculateMigrationComplexity calculates OS and disk complexity breakdowns
// for the given cluster within the assessment's inventory.
// When clusterID is empty, the vCenter-level aggregate inventory is used.
func (es *EstimationService) CalculateMigrationComplexity(
	ctx context.Context,
	assessmentID uuid.UUID,
	clusterIDs []string,
) (*svcmodel.MigrationComplexityResult, error) {
	logger := es.logger.WithContext(ctx)
	tracer := logger.Operation("calculate_migration_complexity").
		WithUUID("assessment_id", assessmentID).
		WithParam("cluster_ids", clusterIDs).
		Build()

	// Fetch and aggregate inventory data
	invData, err := es.getAggregatedInventoryData(ctx, assessmentID, clusterIDs, tracer)
	if err != nil {
		tracer.Error(err).Log()
		return nil, err
	}

	result, err := es.buildComplexityResult(invData)
	if err != nil {
		tracer.Error(err).Log()
		return nil, err
	}

	tracer.Success().Log()
	return result, nil
}

func (es *EstimationService) CalculateMigrationEstimationByComplexity(
	ctx context.Context,
	assessmentID uuid.UUID,
	clusterIDs []string,
	schemas []engines.Schema,
	userParams []estimation.Param,
) (*svcmodel.MigrationEstimationByComplexityResult, error) {
	logger := es.logger.WithContext(ctx)
	tracer := logger.Operation("calculate_migration_estimation_by_complexity").
		WithUUID("assessment_id", assessmentID).
		WithParam("cluster_ids", clusterIDs).
		Build()

	err := engines.ValidateParams(userParams)
	if err != nil {
		return nil, &ErrInvalidEstimationParam{Msg: err.Error()}
	}

	invData, err := es.getAggregatedInventoryData(ctx, assessmentID, clusterIDs, tracer)
	if err != nil {
		tracer.Error(err).Log()
		return nil, err
	}

	baseParams := engines.BuildBaseParams(userParams...)
	result := &svcmodel.MigrationEstimationByComplexityResult{
		Buckets:           buildComplexityByOsDisk(invData.Vms.ComplexityDistribution),
		BucketEstimations: make(svcmodel.BucketEstimations),
		EstimationContext: svcmodel.EstimationContext{Schemas: schemas, BaseParams: baseParams},
		ComplexityMatrix:  complexity.ComplexityMatrix,
	}
	for _, bucket := range result.Buckets {
		if bucket.VMCount == 0 {
			continue
		}
		// The inventory reports TiB; calculators use GiB, as does DiskGB.Total.
		params := append(baseParams,
			estimation.Param{Key: calculators.ParamVMCount, Value: bucket.VMCount},
			estimation.Param{Key: calculators.ParamTotalDiskGB, Value: bucket.TotalSizeTB * 1024},
		)
		schemaResults, err := runEstimation(schemas, params)
		if err != nil {
			tracer.Error(err).Log()
			return nil, err
		}
		result.BucketEstimations[bucket.Score] = make(map[string]*estimation.Result, len(schemaResults))
		for schema, estimate := range schemaResults {
			result.BucketEstimations[bucket.Score][string(schema)] = estimate
		}
	}

	tracer.Success().Log()
	return result, nil
}

// getAggregatedInventoryData fetches the assessment's inventory and returns
// aggregated data for the specified clusters. This is the new entry point for
// multi-cluster complexity calculations.
func (es *EstimationService) getAggregatedInventoryData(
	ctx context.Context,
	assessmentID uuid.UUID,
	clusterIDs []string,
	tracer *log.OperationTracer,
) (svcmodel.InventoryData, error) {
	// Fetch full inventory from database
	inventory, err := loadAssessmentInventory(ctx, es.store, assessmentID)
	if err != nil {
		tracer.Error(err).Log()
		return svcmodel.InventoryData{}, err
	}

	// Aggregate the requested clusters
	invData, err := aggregateInventoryData(&inventory, clusterIDs, assessmentID)
	if err != nil {
		return svcmodel.InventoryData{}, err
	}

	return invData, nil
}

// resolveInventoryData returns the InventoryData for the given clusterID,
// or the vCenter-level aggregate when clusterID is empty.
func resolveInventoryData(inventory svcmodel.Inventory, clusterID string, assessmentID uuid.UUID) (svcmodel.InventoryData, error) {
	if clusterID == "" {
		if inventory.Vcenter == nil {
			return svcmodel.InventoryData{}, fmt.Errorf("inventory has no vcenter-level data")
		}
		return *inventory.Vcenter, nil
	}
	data, ok := inventory.Clusters[clusterID]
	if !ok {
		return svcmodel.InventoryData{}, NewErrClusterNotFound(clusterID, assessmentID)
	}
	return data, nil
}

// resolveEstimationInventoryData returns the combined InventoryData for the given clusterIDs,
// or the vCenter-level aggregate when clusterIDs is empty.
// Empty and duplicate IDs are ignored, but a non-empty list needs at least one usable ID.
func resolveEstimationInventoryData(inventory svcmodel.Inventory, clusterIDs []string, assessmentID uuid.UUID) (svcmodel.InventoryData, error) {
	if len(clusterIDs) == 0 {
		return resolveInventoryData(inventory, "", assessmentID)
	}

	var combined svcmodel.InventoryData
	seen := make(map[string]struct{}, len(clusterIDs))
	for _, clusterID := range clusterIDs {
		if clusterID == "" {
			continue
		}
		if _, exists := seen[clusterID]; exists {
			continue
		}
		seen[clusterID] = struct{}{}

		data, err := resolveInventoryData(inventory, clusterID, assessmentID)
		if err != nil {
			return svcmodel.InventoryData{}, err
		}
		combined.Vms.Total += data.Vms.Total
		combined.Vms.DiskGB.Total += data.Vms.DiskGB.Total
	}
	if len(seen) == 0 {
		return svcmodel.InventoryData{}, NewErrInvalidRequest("cluster IDs must contain at least one non-empty value")
	}
	return combined, nil
}

// buildComplexityResult converts cluster inventory data into complexity breakdowns.
func (es *EstimationService) buildComplexityResult(clusterInventory svcmodel.InventoryData) (*svcmodel.MigrationComplexityResult, error) {
	if clusterInventory.Vms.OsInfo == nil {
		return nil, fmt.Errorf("inventory has no osInfo data")
	}
	// Prefer the dedicated complexity-tier field (new snapshots).
	// Fall back to DiskSizeTier for old snapshots whose labels still match DiskSizeScores.
	diskSource := clusterInventory.Vms.DiskComplexityTier
	if diskSource == nil || len(*diskSource) == 0 {
		diskSource = clusterInventory.Vms.DiskSizeTier
	}
	if diskSource == nil || len(*diskSource) == 0 {
		return nil, fmt.Errorf("inventory has no disk tier data (diskComplexityTier or diskSizeTier)")
	}

	diskEntries := make([]complexity.DiskTierInput, 0, len(*diskSource))
	for label, tier := range *diskSource {
		diskEntries = append(diskEntries, complexity.DiskTierInput{
			Label:       label,
			VMCount:     tier.VmCount,
			TotalSizeTB: tier.TotalSizeTB,
		})
	}

	osEntries := make([]complexity.VMOsEntry, 0, len(*clusterInventory.Vms.OsInfo))
	for osName, info := range *clusterInventory.Vms.OsInfo {
		osEntries = append(osEntries, complexity.VMOsEntry{Name: osName, Count: info.Count})
	}

	return &svcmodel.MigrationComplexityResult{
		ComplexityByOS:     complexity.OSBreakdown(osEntries),
		ComplexityByOSName: complexity.OSNameBreakdown(osEntries),
		ComplexityByDisk:   complexity.DiskBreakdown(diskEntries),
		DiskSizeRatings:    complexity.DiskSizeRangeRatings(),
		OSRatings:          complexity.OSRatings(osEntries),
	}, nil
}

// buildComplexityByOsDisk converts the sparse ComplexityDistribution map (string score → DiskSizeTierSummary)
// into a []OSDiskEntry in canonical score order (0–4). Absent scores carry VMCount == 0 and TotalSizeTB == 0.
func buildComplexityByOsDisk(dist *map[string]svcmodel.DiskSizeTierSummary) []complexity.OSDiskEntry {
	result := make([]complexity.OSDiskEntry, len(complexity.OSScores))
	for i, s := range complexity.OSScores {
		var vmCount int
		var totalSizeTB float64
		if dist != nil {
			key := strconv.Itoa(s)
			if entry, ok := (*dist)[key]; ok {
				vmCount = entry.VmCount
				totalSizeTB = entry.TotalSizeTB
			}
		}
		result[i] = complexity.OSDiskEntry{Score: s, VMCount: vmCount, TotalSizeTB: totalSizeTB}
	}
	return result
}

// aggregateInventoryData merges inventory data from multiple clusters into a single InventoryData.
// When clusterIDs contains a single empty string, it returns the vCenter aggregate.
// When clusterIDs contains one cluster ID, it returns that cluster's data directly.
// When clusterIDs contains multiple cluster IDs, it merges their data.
func aggregateInventoryData(inventory *svcmodel.Inventory, clusterIDs []string, assessmentID uuid.UUID) (svcmodel.InventoryData, error) {
	// Edge case: empty list
	if len(clusterIDs) == 0 {
		return svcmodel.InventoryData{}, fmt.Errorf("at least one cluster ID required")
	}

	// Special case: vCenter aggregate (single empty string)
	if usesVCenterAggregate(clusterIDs) {
		if inventory.Vcenter == nil {
			return svcmodel.InventoryData{}, fmt.Errorf("inventory has no vcenter-level data")
		}
		return *inventory.Vcenter, nil
	}

	// Fast path: single cluster (no aggregation needed)
	if len(clusterIDs) == 1 {
		data, exists := inventory.Clusters[clusterIDs[0]]
		if !exists {
			return svcmodel.InventoryData{}, NewErrClusterNotFound(clusterIDs[0], assessmentID)
		}
		return data, nil
	}

	// Multi-cluster aggregation
	// Build set of unique cluster IDs and validate they exist
	uniqueClusterIDs := make(map[string]bool, len(clusterIDs))
	for _, clusterID := range clusterIDs {
		if _, exists := inventory.Clusters[clusterID]; !exists {
			return svcmodel.InventoryData{}, NewErrClusterNotFound(clusterID, assessmentID)
		}
		uniqueClusterIDs[clusterID] = true
	}

	// Initialize result
	result := svcmodel.InventoryData{
		Vms:   svcmodel.VMs{},
		Infra: svcmodel.Infra{},
	}

	// Initialize maps
	osInfo := make(map[string]svcmodel.OsInfo)

	// Aggregate across unique clusters
	for clusterID := range uniqueClusterIDs {
		cluster := inventory.Clusters[clusterID]

		// Aggregate VM totals
		result.Vms.Total += cluster.Vms.Total
		result.Vms.TotalMigratable += cluster.Vms.TotalMigratable
		if cluster.Vms.TotalMigratableWithWarnings != nil {
			if result.Vms.TotalMigratableWithWarnings == nil {
				result.Vms.TotalMigratableWithWarnings = new(int)
			}
			*result.Vms.TotalMigratableWithWarnings += *cluster.Vms.TotalMigratableWithWarnings
		}

		// Aggregate resource breakdowns
		result.Vms.CpuCores.Total += cluster.Vms.CpuCores.Total
		result.Vms.CpuCores.TotalForMigratable += cluster.Vms.CpuCores.TotalForMigratable
		result.Vms.CpuCores.TotalForMigratableWithWarnings += cluster.Vms.CpuCores.TotalForMigratableWithWarnings
		result.Vms.CpuCores.TotalForNotMigratable += cluster.Vms.CpuCores.TotalForNotMigratable

		result.Vms.RamGB.Total += cluster.Vms.RamGB.Total
		result.Vms.RamGB.TotalForMigratable += cluster.Vms.RamGB.TotalForMigratable
		result.Vms.RamGB.TotalForMigratableWithWarnings += cluster.Vms.RamGB.TotalForMigratableWithWarnings
		result.Vms.RamGB.TotalForNotMigratable += cluster.Vms.RamGB.TotalForNotMigratable

		result.Vms.DiskGB.Total += cluster.Vms.DiskGB.Total
		result.Vms.DiskGB.TotalForMigratable += cluster.Vms.DiskGB.TotalForMigratable
		result.Vms.DiskGB.TotalForMigratableWithWarnings += cluster.Vms.DiskGB.TotalForMigratableWithWarnings
		result.Vms.DiskGB.TotalForNotMigratable += cluster.Vms.DiskGB.TotalForNotMigratable

		// Aggregate OS info
		if cluster.Vms.OsInfo != nil {
			for osName, info := range *cluster.Vms.OsInfo {
				existing := osInfo[osName]
				existing.Count += info.Count
				// Preserve support metadata from any cluster
				if info.Supported {
					existing.Supported = true
				}
				if info.SupportTier != nil && (existing.SupportTier == nil || *info.SupportTier > *existing.SupportTier) {
					existing.SupportTier = info.SupportTier
				}
				if info.UpgradeRecommendation != nil && existing.UpgradeRecommendation == nil {
					existing.UpgradeRecommendation = info.UpgradeRecommendation
				}
				osInfo[osName] = existing
			}
		}

		// Aggregate disk complexity tiers
		diskSource := cluster.Vms.DiskComplexityTier
		if diskSource == nil || len(*diskSource) == 0 {
			diskSource = cluster.Vms.DiskSizeTier // Fallback for old snapshots
		}
		if diskSource != nil {
			if result.Vms.DiskComplexityTier == nil {
				diskComplexityTier := make(map[string]svcmodel.DiskSizeTierSummary)
				result.Vms.DiskComplexityTier = &diskComplexityTier
			}
			for tierLabel, summary := range *diskSource {
				existing := (*result.Vms.DiskComplexityTier)[tierLabel]
				existing.VmCount += summary.VmCount
				existing.TotalSizeTB += summary.TotalSizeTB
				(*result.Vms.DiskComplexityTier)[tierLabel] = existing
			}
		}

		// Aggregate complexity distribution
		if cluster.Vms.ComplexityDistribution != nil {
			if result.Vms.ComplexityDistribution == nil {
				complexityDist := make(map[string]svcmodel.DiskSizeTierSummary)
				result.Vms.ComplexityDistribution = &complexityDist
			}
			for scoreKey, summary := range *cluster.Vms.ComplexityDistribution {
				existing := (*result.Vms.ComplexityDistribution)[scoreKey]
				existing.VmCount += summary.VmCount
				existing.TotalSizeTB += summary.TotalSizeTB
				(*result.Vms.ComplexityDistribution)[scoreKey] = existing
			}
		}
	}

	// Assign aggregated maps only when non-empty to preserve nil-check error handling in buildComplexityResult
	if len(osInfo) > 0 {
		result.Vms.OsInfo = &osInfo
	}

	return result, nil
}

func runEstimation(schemas []engines.Schema, params []estimation.Param) (map[engines.Schema]*estimation.Result, error) {
	engineMap, err := engines.BuildEngines(schemas)
	if err != nil {
		return nil, &ErrInvalidSchema{Msg: err.Error()}
	}

	results := make(map[engines.Schema]*estimation.Result, len(engineMap))
	for schema, engine := range engineMap {
		result := engine.Estimate(params)
		results[schema] = &result
	}
	return results, nil
}

// loadAssessmentInventory reads the main inventory into the service model.
func loadAssessmentInventory(ctx context.Context, s store.Store, id uuid.UUID) (svcmodel.Inventory, error) {
	assessment, err := s.Assessment().Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return svcmodel.Inventory{}, NewErrAssessmentNotFound(id)
		}
		return svcmodel.Inventory{}, fmt.Errorf("failed to get assessment: %w", err)
	}
	if assessment.MainInventory.Inventory.Vcenter == nil && assessment.MainInventory.Inventory.Clusters == nil {
		return svcmodel.Inventory{}, fmt.Errorf("assessment has no full inventory")
	}

	return assessment.MainInventory.Inventory, nil
}
