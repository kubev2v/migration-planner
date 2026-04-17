package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/google/uuid"
	_ "github.com/marcboeker/go-duckdb/v2" // DuckDB driver
	"github.com/riverqueue/river"

	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/duckdb_parser"
	"github.com/kubev2v/migration-planner/pkg/inventory/converters"
	"github.com/kubev2v/migration-planner/pkg/log"
)

// RVToolsWorker processes RVTools assessment jobs.
type RVToolsWorker struct {
	river.WorkerDefaults[RVToolsJobArgs]
	store     store.Store
	validator duckdb_parser.Validator // Shared, stateless
	s3        *config.S3
}

// NewRVToolsWorker creates a new RVTools worker.
func NewRVToolsWorker(store store.Store, validator duckdb_parser.Validator, cfg *config.S3) *RVToolsWorker {
	return &RVToolsWorker{
		store:     store,
		validator: validator,
		s3:        cfg,
	}
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
	parser := duckdb_parser.New(db, w.validator)
	if err := parser.Init(); err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("initializing duckdb schema: %w", err)
	}
	return parser, db, nil
}

func (w *RVToolsWorker) Timeout(_ *river.Job[RVToolsJobArgs]) time.Duration {
	return 10 * time.Minute
}

// failJob logs an error, updates job status to failed, and returns the error.
func (w *RVToolsWorker) failJob(ctx context.Context, logger *log.OperationTracer, jobID int64, step string, err error, errMsg string) error {
	logger.Error(err).WithString("step", step).Log()
	if updateErr := w.updateJobStatus(ctx, jobID, model.JobStatusFailed, errMsg, nil); updateErr != nil {
		logger.Error(updateErr).WithString("step", "update_failed_status").Log()
	}
	return err
}

// Work processes an RVTools assessment job.
func (w *RVToolsWorker) Work(ctx context.Context, job *river.Job[RVToolsJobArgs]) error {
	logger := log.NewDebugLogger("rvtools_worker").
		WithContext(ctx).
		Operation("process_rvtools_job").
		WithParam("job_id", job.ID).
		WithString("assessment_name", job.Args.Name).
		Build()

	logger.Step("job_started").Log()

	client, err := minio.New(w.s3.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(w.s3.AccessKey, w.s3.SecretKey, ""),
		Secure: false,
	})
	if err != nil {
		return fmt.Errorf("failed to create s3 client: %w", err)
	}

	defer func() {
		_ = client.RemoveObject(
			ctx,
			w.s3.RvtoolsBucket,
			job.Args.S3RvtoolsFileName,
			minio.RemoveObjectOptions{},
		)
	}()

	// Write file content to temp file for DuckDB ingestion
	tempFilePath, cleanup, err := w.downloadToTempFile(ctx, client, job.Args.S3RvtoolsFileName)
	defer cleanup()

	// Create per-job DuckDB instance for isolation
	parser, duckDB, err := w.createParser()
	if err != nil {
		return w.failJob(ctx, logger, job.ID, "create_parser", err, fmt.Sprintf("failed to create DuckDB parser: %v", err))
	}
	defer func() { _ = duckDB.Close() }()

	// Update status to validating before ingestion (which includes OPA validation)
	if err := w.updateJobStatus(ctx, job.ID, model.JobStatusValidating, "", nil); err != nil {
		logger.Error(err).WithString("step", "update_validating_status").Log()
	}

	// Ingest RVTools file using duckdb_parser
	validationResult, err := parser.IngestRvTools(ctx, tempFilePath)
	if err != nil {
		return w.failJob(ctx, logger, job.ID, "ingest_rvtools", err, fmt.Sprintf("error ingesting RVTools file: %v", err))
	}

	// Check for validation errors
	if validationResult.HasErrors() {
		validationErr := fmt.Errorf("validation failed: %v", validationResult.Errors)
		return w.failJob(ctx, logger, job.ID, "validate_rvtools", validationErr, fmt.Sprintf("RVTools validation failed: %v", validationResult.Errors[0].Message))
	}

	// Log any warnings
	for _, warning := range validationResult.Warnings {
		logger.Step("validation_warning").WithString("code", warning.Code).WithString("message", warning.Message).Log()
	}

	// Update status to parsing
	if err := w.updateJobStatus(ctx, job.ID, model.JobStatusParsing, "", nil); err != nil {
		logger.Error(err).WithString("step", "update_parsing_status").Log()
	}

	// Build inventory from parsed data
	logger.Step("building_inventory").Log()
	inv, err := parser.BuildInventory(ctx)
	if err != nil {
		return w.failJob(ctx, logger, job.ID, "build_inventory", err, fmt.Sprintf("error building inventory: %v", err))
	}
	inventory := converters.ToAPI(inv)

	// Marshal inventory to JSON
	inventoryJSON, err := json.Marshal(inventory)
	if err != nil {
		return w.failJob(ctx, logger, job.ID, "marshal_inventory", err, fmt.Sprintf("error marshaling inventory: %v", err))
	}

	// Check for cancellation before creating assessment
	if err := ctx.Err(); err != nil {
		logger.Error(err).WithString("step", "pre_create_assessment_cancelled").Log()
		return err
	}

	logger.Step("creating_assessment").Log()

	createdAssessment, err := w.createAssessment(ctx, job.Args, inventoryJSON)
	if err != nil {
		logger.Error(err).WithString("step", "save_assessment").Log()
		return err
	}

	// Update job with assessment ID
	if err := w.updateJobStatus(ctx, job.ID, model.JobStatusCompleted, "", &createdAssessment.ID); err != nil {
		logger.Error(err).WithString("step", "update_completed_status").Log()
	}

	logger.Success().
		WithUUID("assessment_id", createdAssessment.ID).
		WithString("assessment_name", createdAssessment.Name).
		Log()

	return nil
}

func (w *RVToolsWorker) downloadToTempFile(
	ctx context.Context,
	client *minio.Client,
	S3FileName string,
) (string, func(), error) {

	tempFile, err := os.CreateTemp("", "rvtools-*.xlsx")
	if err != nil {
		return "", nil, err
	}

	cleanup := func() {
		_ = os.Remove(tempFile.Name())
	}

	obj, err := client.GetObject(ctx, w.s3.RvtoolsBucket, S3FileName, minio.GetObjectOptions{})
	if err != nil {
		cleanup()
		return "", nil, err
	}
	defer func() {
		_ = obj.Close()
	}()

	if _, err := io.Copy(tempFile, obj); err != nil {
		cleanup()
		return "", nil, err
	}

	_ = tempFile.Close()

	return tempFile.Name(), cleanup, nil
}

func (w *RVToolsWorker) createAssessment(
	ctx context.Context,
	args RVToolsJobArgs,
	inventoryJSON []byte,
) (*model.Assessment, error) {
	// Build assessment model
	assessment := model.Assessment{
		ID:         uuid.New(),
		Name:       args.Name,
		OrgID:      args.OrgID,
		Username:   args.Username,
		SourceType: "rvtools",
	}
	if args.FirstName != "" {
		assessment.OwnerFirstName = &args.FirstName
	}
	if args.LastName != "" {
		assessment.OwnerLastName = &args.LastName
	}

	createdAssessment, err := w.store.Assessment().Create(ctx, assessment, inventoryJSON)
	if err != nil {
		var errMsg string
		if errors.Is(err, store.ErrDuplicateKey) {
			// Same format as service.NewErrAssessmentDuplicateName
			errMsg = fmt.Sprintf("assessment with name '%s' already exists", assessment.Name)
		} else {
			errMsg = fmt.Sprintf("failed to create assessment: %v", err)
		}
		return nil, fmt.Errorf(errMsg)
	}

	updates := store.NewRelationshipBuilder().
		With(model.NewAssessmentResource(assessment.ID.String()), model.OwnerRelation, model.NewUserSubject(args.Username)).
		Build()

	if err := w.store.Authz().WriteRelationships(ctx, updates); err != nil {
		return nil, fmt.Errorf("authz: failed to write owner relation: %w", err)
	}

	return createdAssessment, nil
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
