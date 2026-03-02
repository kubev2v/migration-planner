package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/pkg/estimations/complexity"
	"github.com/kubev2v/migration-planner/pkg/estimations/estimation"
	"github.com/kubev2v/migration-planner/pkg/estimations/estimation/calculators"
	"github.com/kubev2v/migration-planner/pkg/log"
)

// MigrationComplexityResult holds the output of a complexity estimation run.
type MigrationComplexityResult struct {
	ComplexityByDisk []complexity.DiskComplexityEntry // scores 1–4, always 4 entries
	ComplexityByOS   []complexity.OSDifficultyEntry   // scores 0–4, always 5 entries
	DiskSizeRatings  map[string]complexity.Score      // static tier label → score lookup
	OSRatings        map[string]complexity.Score      // per-inventory OS name → score
}

// MigrationAssessmentResult represents the result of a migration assessment calculation
type MigrationAssessmentResult struct {
	TotalDuration time.Duration
	Breakdown     map[string]estimation.Estimation
}

// EstimationService orchestrates the migration time estimation workflow.
// It retrieves assessment and inventory data from the store and runs them
// through the estimation Engine to produce a MigrationAssessmentResult.
type EstimationService struct {
	store  store.Store
	engine *estimation.Engine
	logger *log.StructuredLogger
}

// NewEstimationService creates an EstimationService with the default set of calculators registered.
func NewEstimationService(store store.Store) *EstimationService {
	engine := estimation.NewEngine()

	// Register calculators
	// TODO: later phases can make this configurable by the user
	engine.Register(calculators.NewStorageMigration())
	engine.Register(calculators.NewPostMigrationTroubleShooting())

	return &EstimationService{
		store:  store,
		engine: engine,
		logger: log.NewDebugLogger("estimation_service"),
	}
}

// CalculateMigrationEstimation calculates migration time estimation for a given assessment and cluster
func (es *EstimationService) CalculateMigrationEstimation(
	ctx context.Context,
	assessmentID uuid.UUID,
	clusterID string,
) (*MigrationAssessmentResult, error) {
	logger := es.logger.WithContext(ctx)
	tracer := logger.Operation("calculate_migration_estimation").
		WithUUID("assessment_id", assessmentID).
		WithString("cluster_id", clusterID).
		Build()

	assessment, err := es.store.Assessment().Get(ctx, assessmentID)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			tracer.Error(err).Log()
			return nil, NewErrAssessmentNotFound(assessmentID)
		}
		tracer.Error(err).Log()
		return nil, fmt.Errorf("failed to get assessment: %w", err)
	}

	if len(assessment.Snapshots) == 0 {
		err := fmt.Errorf("assessment has no snapshots")
		tracer.Error(err).Log()
		return nil, err
	}

	// assuming each assessment has one snapshot at most
	latestSnapshot := assessment.Snapshots[0]
	if len(latestSnapshot.Inventory) == 0 {
		err := fmt.Errorf("latest snapshot has empty inventory")
		tracer.Error(err).Log()
		return nil, err
	}

	var inventory api.Inventory
	if err := json.Unmarshal(latestSnapshot.Inventory, &inventory); err != nil {
		tracer.Error(err).Log()
		return nil, fmt.Errorf("failed to parse inventory: %w", err)
	}

	if len(inventory.Clusters) == 0 {
		err := fmt.Errorf("inventory has no clusters")
		tracer.Error(err).Log()
		return nil, err
	}

	clusterInventory, exists := inventory.Clusters[clusterID]
	if !exists {
		err := NewErrClusterNotFound(clusterID, assessmentID)
		tracer.Error(err).Log()
		return nil, err
	}

	params := es.mapClusterToParams(clusterInventory)

	tracer.Step("mapped_params").WithInt("param_count", len(params)).Log()

	results := es.engine.Run(params)

	// Calculate total duration (simple sum for now)
	totalDuration := time.Duration(0)
	for _, est := range results {
		totalDuration += est.Duration
	}

	tracer.Success().
		WithString("total_duration", totalDuration.String()).
		WithInt("calculator_count", len(results)).
		Log()

	return &MigrationAssessmentResult{
		TotalDuration: totalDuration,
		Breakdown:     results,
	}, nil
}

// CalculateMigrationComplexity calculates OS and disk complexity breakdowns
// for the given cluster within the assessment's inventory.
func (es *EstimationService) CalculateMigrationComplexity(
	ctx context.Context,
	assessmentID uuid.UUID,
	clusterID string,
) (*MigrationComplexityResult, error) {
	logger := es.logger.WithContext(ctx)
	tracer := logger.Operation("calculate_migration_complexity").
		WithUUID("assessment_id", assessmentID).
		WithString("cluster_id", clusterID).
		Build()

	assessment, err := es.store.Assessment().Get(ctx, assessmentID)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			tracer.Error(err).Log()
			return nil, NewErrAssessmentNotFound(assessmentID)
		}
		tracer.Error(err).Log()
		return nil, fmt.Errorf("failed to get assessment: %w", err)
	}

	if len(assessment.Snapshots) == 0 {
		err := fmt.Errorf("assessment has no snapshots")
		tracer.Error(err).Log()
		return nil, err
	}

	latestSnapshot := assessment.Snapshots[0]
	if len(latestSnapshot.Inventory) == 0 {
		err := fmt.Errorf("latest snapshot has empty inventory")
		tracer.Error(err).Log()
		return nil, err
	}

	var inventory api.Inventory
	if err := json.Unmarshal(latestSnapshot.Inventory, &inventory); err != nil {
		tracer.Error(err).Log()
		return nil, fmt.Errorf("failed to parse inventory: %w", err)
	}

	if len(inventory.Clusters) == 0 {
		err := fmt.Errorf("inventory has no clusters")
		tracer.Error(err).Log()
		return nil, err
	}

	clusterInventory, exists := inventory.Clusters[clusterID]
	if !exists {
		err := NewErrClusterNotFound(clusterID, assessmentID)
		tracer.Error(err).Log()
		return nil, err
	}

	result, err := es.buildComplexityResult(clusterInventory)
	if err != nil {
		tracer.Error(err).Log()
		return nil, err
	}

	tracer.Success().Log()
	return result, nil
}

// buildComplexityResult converts cluster inventory data into complexity breakdowns.
func (es *EstimationService) buildComplexityResult(clusterInventory api.InventoryData) (*MigrationComplexityResult, error) {
	if clusterInventory.Vms.OsInfo == nil {
		return nil, fmt.Errorf("inventory has no osInfo data")
	}
	if clusterInventory.Vms.DiskSizeTier == nil {
		return nil, fmt.Errorf("inventory has no diskSizeTier data")
	}

	osEntries := make([]complexity.VMOsEntry, 0, len(*clusterInventory.Vms.OsInfo))
	for osName, info := range *clusterInventory.Vms.OsInfo {
		osEntries = append(osEntries, complexity.VMOsEntry{Name: osName, Count: info.Count})
	}

	diskEntries := make([]complexity.DiskTierInput, 0, len(*clusterInventory.Vms.DiskSizeTier))
	for label, tier := range *clusterInventory.Vms.DiskSizeTier {
		diskEntries = append(diskEntries, complexity.DiskTierInput{
			Label:       label,
			VMCount:     tier.VmCount,
			TotalSizeTB: tier.TotalSizeTB,
		})
	}

	return &MigrationComplexityResult{
		ComplexityByOS:   complexity.OSBreakdown(osEntries),
		ComplexityByDisk: complexity.DiskBreakdown(diskEntries),
		DiskSizeRatings:  complexity.DiskSizeRangeRatings(),
		OSRatings:        complexity.OSRatings(osEntries),
	}, nil
}

// mapClusterToParams converts cluster inventory data to estimation parameters
func (es *EstimationService) mapClusterToParams(clusterInventory api.InventoryData) []estimation.Param {
	params := []estimation.Param{}

	// Extract total disk GB from cluster VMs
	totalDiskGB := clusterInventory.Vms.DiskGB.Total
	params = append(params, estimation.Param{
		Key:   calculators.ParamTotalDiskGB,
		Value: float64(totalDiskGB),
	})

	// Extract total VM count
	totalVMs := clusterInventory.Vms.Total
	params = append(params, estimation.Param{
		Key:   calculators.ParamVMCount,
		Value: totalVMs,
	})

	// TODO: expose via API request body in a future phase
	params = append(params, estimation.Param{
		Key:   calculators.ParamTransferRateMbps,
		Value: calculators.DefaultTransferRateMbps,
	})

	// TODO: expose via API request body in a future phase
	params = append(params, estimation.Param{
		Key:   calculators.ParamWorkHoursPerDay,
		Value: calculators.DefaultWorkHoursPerDay,
	})

	return params
}
