package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	_ "github.com/marcboeker/go-duckdb/v2" // DuckDB driver
	"github.com/riverqueue/river"

	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	svcmodel "github.com/kubev2v/migration-planner/internal/service/model"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/duckdb_parser"
	"github.com/kubev2v/migration-planner/pkg/log"
	pkgstore "github.com/kubev2v/migration-planner/pkg/store"
)

type RVToolsWorker struct {
	river.WorkerDefaults[model.RVToolsJobArgs]
	store         store.Store
	assessmentSvc service.AssessmentServicer
	validator     duckdb_parser.Validator
}

func NewRVToolsWorker(store store.Store, assessmentSvc service.AssessmentServicer, validator duckdb_parser.Validator) *RVToolsWorker {
	return &RVToolsWorker{
		store:         store,
		assessmentSvc: assessmentSvc,
		validator:     validator,
	}
}

// Work processes an RVTools assessment job.
func (w *RVToolsWorker) Work(ctx context.Context, job *river.Job[model.RVToolsJobArgs]) error {
	logger := log.NewDebugLogger("rvtools_worker").
		WithContext(ctx).
		Operation("process_rvtools_job").
		WithParam("job_id", job.ID).
		WithString("assessment_name", job.Args.Name).
		Build()

	logger.Step("job_started").Log()

	// Create per-job DuckDB instance for isolation
	parser, duckDB, err := w.createParser()
	if err != nil {
		return w.failJob(ctx, logger, job.ID, "create_parser", err, fmt.Sprintf("failed to create DuckDB parser: %v", err))
	}
	defer func() {
		_ = duckDB.Close()
		_ = os.Remove(job.Args.File)
	}()

	// Update status to parsing
	if err := w.updateJobStatus(ctx, job.ID, model.JobStatusParsing, "", nil); err != nil {
		logger.Error(err).WithString("step", "update_parsing_status").Log()
	}

	parsingErr := NewRVToolParsingError()
	if err := ctx.Err(); err != nil {
		return err
	}

	result, err := w.parseFile(ctx, parser, job.Args.File, logger)
	if err != nil {
		parsingErr.Add(job.Args.Name, err)
	}

	if cleanupErr := parser.Cleanup(); cleanupErr != nil {
		logger.Error(cleanupErr).WithString("step", "cleanup").WithString("file", job.Args.File).Log()
		parsingErr.Add(job.Args.Name, err)
	}

	if parsingErr.HasErrors() {
		return w.failJob(ctx, logger, job.ID, "parse_files", parsingErr, parsingErr.Error())
	}

	// Check for cancellation before creating assessment
	if err := ctx.Err(); err != nil {
		logger.Error(err).WithString("step", "pre_create_assessment_cancelled").Log()
		return err
	}

	switch job.Args.OperationType {
	case model.CreateOperationType:
		logger.Step("creating_assessment").Log()
		assessmentID := uuid.New()

		createForm := mappers.AssessmentCreateForm{
			ID:         assessmentID,
			Name:       job.Args.Name,
			OrgID:      job.Args.OrgID,
			Username:   job.Args.Username,
			SourceType: "rvtools",
			Inventory:  mappers.AssessmentInventoryFromInventory(uuid.New(), assessmentID, job.Args.Name, false, *result),
		}
		if job.Args.FirstName != "" {
			createForm.OwnerFirstName = &job.Args.FirstName
		}
		if job.Args.LastName != "" {
			createForm.OwnerLastName = &job.Args.LastName
		}

		svcCtx := auth.NewTokenContext(ctx, auth.User{
			Username:     job.Args.Username,
			Organization: job.Args.OrgID,
		})
		createdAssessment, err := w.assessmentSvc.CreateAssessment(svcCtx, createForm)
		if err != nil {
			return w.failJob(ctx, logger, job.ID, "create_assessment", err, fmt.Sprintf("failed to create assessment: %v", err))
		}

		if err := w.updateJobStatus(ctx, job.ID, model.JobStatusCompleted, "", &createdAssessment.ID); err != nil {
			logger.Error(err).WithString("step", "update_completed_status").Log()
		}

		logger.Success().
			WithUUID("assessment_id", createdAssessment.ID).
			WithString("assessment_name", createdAssessment.Name).
			Log()
	case model.UpdateOperationType:
		// Add the new file as a subset inventory to the existing assessment. If a subset
		// with the same vCenter ID already exists, update it in place. Then re-aggregate
		// all subsets into the main inventory (is_subset=false) and persist everything
		// in a single transaction.
		// Assessment itself is updated to update the "update_at" field.
		logger.Step("updating_assessment").Log()

		invBytes, err := json.Marshal(result)
		if err != nil {
			return w.failJob(ctx, logger, job.ID, "marshal_inventory", err, fmt.Sprintf("failed to marshal inventory model: %v", err))
		}

		subsetInv, err := model.NewAssessmentInventory(uuid.New(), job.Args.AssessmentID, job.Args.Name, true, invBytes)
		if err != nil {
			return w.failJob(ctx, logger, job.ID, "build_inventory_model", err, fmt.Sprintf("failed to build inventory model: %v", err))
		}

		assessment, err := w.store.Assessment().Get(ctx, job.Args.AssessmentID)
		if err != nil {
			return w.failJob(ctx, logger, job.ID, "get_assessment", err, fmt.Sprintf("failed to get assessment: %v", err))
		}

		for _, existing := range assessment.Inventories {
			if existing.IsSubset && existing.VCenterID == subsetInv.VCenterID {
				subsetInv.ID = existing.ID
				break
			}
		}

		txCtx, err := w.store.NewTransactionContext(ctx)
		if err != nil {
			return w.failJob(ctx, logger, job.ID, "begin_transaction", err, fmt.Sprintf("failed to begin transaction: %v", err))
		}
		defer func() {
			_, _ = store.Rollback(txCtx)
		}()

		if err := w.store.Assessment().UpsertInventories(txCtx, subsetInv); err != nil {
			return w.failJob(ctx, logger, job.ID, "upsert_subset_inventory", err, fmt.Sprintf("failed to upsert subset inventory: %v", err))
		}

		svcCtx := auth.NewTokenContext(txCtx, auth.User{
			Username:     job.Args.Username,
			Organization: job.Args.OrgID,
		})

		mainInv, err := w.assessmentSvc.AggregateInventories(svcCtx, job.Args.AssessmentID)
		if err != nil {
			return w.failJob(ctx, logger, job.ID, "aggregate_inventories", err, fmt.Sprintf("failed to aggregate inventories: %v", err))
		}

		storeMainInv, err := mappers.AssessmentInventoryToStore(mainInv)
		if err != nil {
			return w.failJob(ctx, logger, job.ID, "aggregate_inventories", err, fmt.Sprintf("failed to convert aggregated inventory: %v", err))
		}

		if err := w.store.Assessment().UpsertInventories(txCtx, storeMainInv); err != nil {
			return w.failJob(ctx, logger, job.ID, "upsert_main_inventory", err, fmt.Sprintf("failed to upsert main inventory: %v", err))
		}

		if _, err := w.store.Assessment().Update(txCtx, job.Args.AssessmentID, &assessment.Name, nil); err != nil {
			return w.failJob(ctx, logger, job.ID, "update_assessment_timestamp", err, fmt.Sprintf("failed to update assessment: %v", err))
		}

		if _, err := store.Commit(txCtx); err != nil {
			return w.failJob(ctx, logger, job.ID, "commit_transaction", err, fmt.Sprintf("failed to commit transaction: %v", err))
		}

		if err := w.updateJobStatus(ctx, job.ID, model.JobStatusCompleted, "", &job.Args.AssessmentID); err != nil {
			logger.Error(err).WithString("step", "update_completed_status").Log()
		}

		logger.Success().
			WithUUID("assessment_id", job.Args.AssessmentID).
			Log()
	default:
		return w.failJob(ctx, logger, job.ID, "parse_files", parsingErr, "invalid operation type")
	}

	return nil
}

func (w *RVToolsWorker) Timeout(_ *river.Job[model.RVToolsJobArgs]) time.Duration {
	return 10 * time.Minute
}

// createParser creates a new per-job DuckDB instance and parser.
// The caller is responsible for closing the returned *sql.DB when done.
func (w *RVToolsWorker) createParser() (*duckdb_parser.Parser, *sql.DB, error) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		return nil, nil, fmt.Errorf("opening duckdb: %w", err)
	}
	extensionDir := "/tmp/duckdb_extensions"
	if _, err := db.Exec(fmt.Sprintf("SET extension_directory='%s';", extensionDir)); err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("setting duckdb extension directory: %w", err)
	}
	if _, err := db.Exec("SET memory_limit='1GB';"); err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("setting duckdb memory limit: %w", err)
	}
	qi := pkgstore.NewQueryInterceptor(db)
	parser := duckdb_parser.New(qi, w.validator)
	if err := parser.Init(); err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("initializing duckdb schema: %w", err)
	}
	return parser, db, nil
}

// failJob logs an error, updates job status to failed, and returns the error.
func (w *RVToolsWorker) failJob(ctx context.Context, logger *log.OperationTracer, jobID int64, step string, err error, errMsg string) error {
	logger.Error(err).WithString("step", step).Log()
	if updateErr := w.updateJobStatus(ctx, jobID, model.JobStatusFailed, errMsg, nil); updateErr != nil {
		logger.Error(updateErr).WithString("step", "update_failed_status").Log()
	}
	return err
}

func (w *RVToolsWorker) parseFile(ctx context.Context, parser *duckdb_parser.Parser, filepath string, logger *log.OperationTracer) (*svcmodel.Inventory, error) {
	// Ingest RVTools file using duckdb_parser
	validationResult, err := parser.IngestRvTools(ctx, filepath)
	if err != nil {
		return nil, err
	}

	// Check for validation errors
	if validationResult.HasErrors() {
		validationErr := fmt.Errorf("validation failed: %v", validationResult.Errors)
		return nil, validationErr
	}

	// Log any warnings
	for _, warning := range validationResult.Warnings {
		logger.Step("validation_warning").WithString("code", warning.Code).WithString("message", warning.Message).Log()
	}

	// Build inventory from parsed data
	logger.Step("building_inventory").Log()
	inv, err := parser.BuildInventory(ctx, nil)
	if err != nil {
		return nil, err
	}

	r := mappers.DuckdbToModel(inv)

	return r, nil
}

// updateJobStatus updates the job's metadata with the current status using job store.
func (w *RVToolsWorker) updateJobStatus(ctx context.Context, jobID int64, status, errorMsg string, assessmentID *uuid.UUID) error {
	metadata := model.RVToolsJobMetadata{
		Status:       status,
		Error:        errorMsg,
		AssessmentID: assessmentID,
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshaling metadata: %w", err)
	}

	return w.store.Job().UpdateMetadata(ctx, jobID, metadataJSON)
}
