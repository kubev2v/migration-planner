package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"github.com/kubev2v/migration-planner/internal/opa"
	"github.com/kubev2v/migration-planner/internal/rvtools"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/log"
)

// RVToolsWorker processes RVTools assessment jobs.
type RVToolsWorker struct {
	river.WorkerDefaults[RVToolsJobArgs]
	opaValidator *opa.Validator
	store        store.Store
}

// NewRVToolsWorker creates a new RVTools worker.
func NewRVToolsWorker(opaValidator *opa.Validator, store store.Store) *RVToolsWorker {
	return &RVToolsWorker{
		opaValidator: opaValidator,
		store:        store,
	}
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

	// Update status to parsing
	if err := w.updateJobStatus(ctx, job.ID, model.JobStatusParsing, "", nil); err != nil {
		logger.Error(err).WithString("step", "update_parsing_status").Log()
		return err
	}

	logger.Step("parsing_rvtools").WithInt("file_size", len(job.Args.FileContent)).Log()

	// Create status callback for ParseRVTools to update status during validation
	statusCallback := func(status string) error {
		return w.updateJobStatus(ctx, job.ID, status, "", nil)
	}

	// Parse RVTools file with callback for status updates
	inventory, err := rvtools.ParseRVTools(ctx, job.Args.FileContent, w.opaValidator, statusCallback)
	if err != nil {
		errMsg := fmt.Sprintf("error parsing RVTools file: %v", err)
		logger.Error(err).WithString("step", "parse_rvtools").Log()
		if updateErr := w.updateJobStatus(ctx, job.ID, model.JobStatusFailed, errMsg, nil); updateErr != nil {
			logger.Error(updateErr).WithString("step", "update_failed_status").Log()
		}
		return err
	}

	// Check for cancellation before creating assessment
	if err := ctx.Err(); err != nil {
		logger.Error(err).WithString("step", "pre_create_assessment_cancelled").Log()
		return err
	}

	logger.Step("creating_assessment").Log()

	// Build assessment model
	assessment := model.Assessment{
		ID:         uuid.New(),
		Name:       job.Args.Name,
		OrgID:      job.Args.OrgID,
		Username:   job.Args.Username,
		SourceType: "rvtools",
	}
	if job.Args.FirstName != "" {
		assessment.OwnerFirstName = &job.Args.FirstName
	}
	if job.Args.LastName != "" {
		assessment.OwnerLastName = &job.Args.LastName
	}

	createdAssessment, err := w.store.Assessment().Create(ctx, assessment, inventory)
	if err != nil {
		var errMsg string
		if errors.Is(err, store.ErrDuplicateKey) {
			// Same format as service.NewErrAssessmentDuplicateName
			errMsg = fmt.Sprintf("assessment with name '%s' already exists", assessment.Name)
		} else {
			errMsg = fmt.Sprintf("failed to create assessment: %v", err)
		}
		logger.Error(err).WithString("step", "create_assessment").Log()
		if updateErr := w.updateJobStatus(ctx, job.ID, model.JobStatusFailed, errMsg, nil); updateErr != nil {
			logger.Error(updateErr).WithString("step", "update_failed_status").Log()
		}
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
