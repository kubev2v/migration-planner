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
	"github.com/kubev2v/migration-planner/pkg/estimations/engines"
	"github.com/kubev2v/migration-planner/pkg/estimations/estimation"
	"github.com/kubev2v/migration-planner/pkg/estimations/estimation/calculators"
	"github.com/kubev2v/migration-planner/pkg/log"
)

// MigrationComplexityResult holds the output of a complexity estimation run.
type MigrationComplexityResult struct {
	ComplexityByDisk   []complexity.DiskComplexityEntry // scores 1–4, always 4 entries
	ComplexityByOS     []complexity.OSDifficultyEntry   // scores 0–4, always 5 entries
	ComplexityByOSName []complexity.OSNameEntry         // one entry per distinct OS name
	DiskSizeRatings    map[string]complexity.Score      // static tier label → score lookup
	OSRatings          map[string]complexity.Score      // per-inventory OS name → score
}

// MigrationAssessmentResult represents the result of a migration assessment calculation
type MigrationAssessmentResult struct {
	MinTotalDuration time.Duration
	MaxTotalDuration time.Duration
	Breakdown        map[string]estimation.Estimation
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

// CalculateMigrationEstimation calculates migration time estimation for a given assessment and cluster
func (es *EstimationService) CalculateMigrationEstimation(
	ctx context.Context,
	assessmentID uuid.UUID,
	clusterID string,
	schemas []engines.Schema,
	userParams []estimation.Param,
) (map[engines.Schema]*MigrationAssessmentResult, error) {
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

	params := mergeParams(
		defaultParams(),
		es.mapClusterToParams(clusterInventory),
		userParams,
	)

	tracer.Step("mapped_params").WithInt("param_count", len(params)).Log()

	engineMap, err := engines.BuildEngines(schemas)
	if err != nil {
		return nil, &ErrInvalidSchema{Msg: err.Error()}
	}

	results := make(map[engines.Schema]*MigrationAssessmentResult, len(engineMap))
	for schema, engine := range engineMap {
		breakdown := engine.Run(params)
		var minTotal, maxTotal time.Duration
		for _, est := range breakdown {
			if est.IsRanged() {
				minTotal += *est.MinDuration
				maxTotal += *est.MaxDuration
			} else {
				minTotal += *est.Duration
				maxTotal += *est.Duration
			}
		}
		results[schema] = &MigrationAssessmentResult{
			MinTotalDuration: minTotal,
			MaxTotalDuration: maxTotal,
			Breakdown:        breakdown,
		}
	}

	tracer.Success().
		WithInt("schema_count", len(results)).
		Log()

	return results, nil
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
		ComplexityByOS:     complexity.OSBreakdown(osEntries),
		ComplexityByOSName: complexity.OSNameBreakdown(osEntries),
		ComplexityByDisk:   complexity.DiskBreakdown(diskEntries),
		DiskSizeRatings:    complexity.DiskSizeRangeRatings(),
		OSRatings:          complexity.OSRatings(osEntries),
	}, nil
}

// ParamDefinition describes a single calculator parameter that can be supplied
// by the user to override the default or inventory-derived value.
//
// This slice is the single source of truth for two things:
//  1. defaultParams() — derives the baseline []estimation.Param from it, so
//     adding a new parameter here automatically makes it part of the merge.
//  2. A future metadata endpoint (e.g. GET /api/v1/estimation/params) that
//     lets the UI render dynamic input forms with correct display names, types,
//     units, and valid ranges without hard-coding any of that in the frontend.
//
// When adding a new calculator parameter, add its definition here first.
type ParamDefinition struct {
	Key         string           // matches estimation.Param.Key and the calculator constant
	DisplayName string           // human-readable label for UI forms
	Type        string           // "number" or "integer"
	Unit        string           // e.g. "Mbps", "hours", "minutes", "" if unitless
	Min         *float64         // inclusive lower bound, nil if unbounded
	Max         *float64         // inclusive upper bound, nil if unbounded
	Default     any              // value used when neither inventory nor user supplies this key
	Schemas     []engines.Schema // schemas that use this parameter; nil means all schemas
}

// estimationParamDefs is the authoritative list of user-overridable calculator parameters.
// See ParamDefinition for the contract each field must satisfy.
var estimationParamDefs = func() []ParamDefinition {
	minRate := 0.1 // Mbps — prevent division by zero in StorageMigration
	minHours := 0.5
	minMins := 1.0
	minEngineers := 1.0
	return []ParamDefinition{
		{
			Key:         calculators.ParamTransferRateMbps,
			DisplayName: "Network Transfer Rate",
			Type:        "number",
			Unit:        "Mbps",
			Min:         &minRate,
			Default:     calculators.DefaultTransferRateMbps,
			Schemas:     []engines.Schema{engines.SchemaNetworkBased},
		},
		{
			Key:         calculators.ParamWorkHoursPerDay,
			DisplayName: "Work Hours per Day",
			Type:        "number",
			Unit:        "hours",
			Min:         &minHours,
			Default:     calculators.DefaultWorkHoursPerDay,
		},
		{
			Key:         calculators.ParamTroubleshootMinsPerVM,
			DisplayName: "Troubleshooting Time per VM",
			Type:        "number",
			Unit:        "minutes",
			Min:         &minMins,
			Default:     calculators.DefaultTroubleshootMinsPerVM,
		},
		{
			Key:         calculators.ParamPostMigrationEngineers,
			DisplayName: "Post-Migration Engineers",
			Type:        "integer",
			Unit:        "",
			Min:         &minEngineers,
			Default:     calculators.DefaultEngineerCount,
		},
	}
}()

// defaultParams derives the baseline []estimation.Param from estimationParamDefs.
// Adding a new parameter to estimationParamDefs automatically includes it here.
func defaultParams() []estimation.Param {
	params := make([]estimation.Param, len(estimationParamDefs))
	for i, def := range estimationParamDefs {
		params[i] = estimation.Param{Key: def.Key, Value: def.Default}
	}
	return params
}

// mergeParams merges parameter layers left-to-right; later layers win on key conflicts.
func mergeParams(layers ...[]estimation.Param) []estimation.Param {
	merged := make(map[string]estimation.Param)
	for _, layer := range layers {
		for _, p := range layer {
			merged[p.Key] = p
		}
	}
	result := make([]estimation.Param, 0, len(merged))
	for _, p := range merged {
		result = append(result, p)
	}
	return result
}

// mapClusterToParams converts cluster inventory data to estimation parameters
func (es *EstimationService) mapClusterToParams(clusterInventory api.InventoryData) []estimation.Param {
	return []estimation.Param{
		{Key: calculators.ParamTotalDiskGB, Value: float64(clusterInventory.Vms.DiskGB.Total)},
		{Key: calculators.ParamVMCount, Value: clusterInventory.Vms.Total},
	}
}
