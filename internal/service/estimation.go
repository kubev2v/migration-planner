package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/estimation"
	"github.com/kubev2v/migration-planner/internal/estimation/calculators"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/pkg/log"
)

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

	return params
}
