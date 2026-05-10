package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/rvtools/jobs"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/log"
)

// JobService handles job-related operations.
type JobService struct {
	riverClient      *river.Client[pgx.Tx]
	jobStore         store.Job
	rvtoolsFileStore store.RVToolsFile
	logger           *log.StructuredLogger
}

// NewJobService creates a new job service.
func NewJobService(store store.Store, riverClient *river.Client[pgx.Tx]) *JobService {
	return &JobService{
		riverClient:      riverClient,
		jobStore:         store.Job(),
		rvtoolsFileStore: store.RVToolsFile(),
		logger:           log.NewDebugLogger("job_service"),
	}
}

// CreateRVToolsJob stores the file content in the rvtools_files table and creates a River job referencing it.
func (s *JobService) CreateRVToolsJob(ctx context.Context, args jobs.RVToolsJobArgs, fileContent []byte) (*v1alpha1.Job, error) {
	logger := s.logger.WithContext(ctx)
	tracer := logger.Operation("create_rvtools_job").
		WithString("name", args.Name).
		WithString("org_id", args.OrgID).
		WithString("username", args.Username).
		Build()

	// Store file content in dedicated bytea table
	fileID := uuid.New()
	if err := s.rvtoolsFileStore.Create(ctx, fileID, fileContent); err != nil {
		tracer.Error(err).Log()
		return nil, fmt.Errorf("storing rvtools file: %w", err)
	}
	args.FileID = fileID.String()

	// Insert job into River (args now contains only the file reference, not the file content)
	insertedJob, err := s.riverClient.Insert(ctx, args, nil)
	if err != nil {
		// Clean up stored file on job insert failure
		_ = s.rvtoolsFileStore.Delete(ctx, fileID)
		tracer.Error(err).Log()
		return nil, fmt.Errorf("inserting job: %w", err)
	}

	tracer.Success().WithParam("job_id", insertedJob.Job.ID).Log()

	jobForm := mappers.JobForm{
		ID:       insertedJob.Job.ID,
		State:    insertedJob.Job.State,
		Metadata: model.RVToolsJobMetadata{},
	}
	return jobForm.ToAPIJob(), nil
}

// GetJob retrieves a job by ID.
func (s *JobService) GetJob(ctx context.Context, jobID int64, orgID, username string) (*v1alpha1.Job, error) {
	logger := s.logger.WithContext(ctx)
	tracer := logger.Operation("get_job").
		WithParam("job_id", jobID).
		Build()

	// Query job from store
	jobRow, err := s.jobStore.Get(ctx, jobID)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return nil, NewErrJobNotFound(jobID)
		}
		tracer.Error(err).Log()
		return nil, fmt.Errorf("querying job: %w", err)
	}

	// Parse args to verify ownership
	var args jobs.RVToolsJobArgs
	if err := json.Unmarshal(jobRow.ArgsJSON, &args); err != nil {
		tracer.Error(err).Log()
		return nil, fmt.Errorf("parsing job args: %w", err)
	}

	// Verify ownership
	if args.OrgID != orgID || args.Username != username {
		return nil, NewErrJobForbidden(jobID)
	}

	// Parse metadata
	var metadata model.RVToolsJobMetadata
	if len(jobRow.MetadataJSON) > 0 {
		// Ignore errors, use empty metadata if parsing fails
		_ = json.Unmarshal(jobRow.MetadataJSON, &metadata)
	}

	jobForm := mappers.JobForm{
		ID:       jobRow.ID,
		State:    jobRow.State,
		Metadata: metadata,
	}
	job := jobForm.ToAPIJob()
	tracer.Success().WithString("status", string(job.Status)).Log()

	return job, nil
}

// CancelJob cancels a job by ID.
func (s *JobService) CancelJob(ctx context.Context, jobID int64, orgID, username string) (*v1alpha1.Job, error) {
	logger := s.logger.WithContext(ctx)
	tracer := logger.Operation("cancel_job").
		WithParam("job_id", jobID).
		Build()

	// First verify ownership by getting the job
	job, err := s.GetJob(ctx, jobID, orgID, username)
	if err != nil {
		return nil, err
	}

	// Check if job can be cancelled
	if job.Status == v1alpha1.JobStatusCompleted || job.Status == v1alpha1.JobStatusFailed || job.Status == v1alpha1.JobStatusCancelled {
		return job, nil // Already in terminal state
	}

	// Cancel the job using JobCancel
	_, err = s.riverClient.JobCancel(ctx, jobID)
	if err != nil {
		tracer.Error(err).Log()
		return nil, fmt.Errorf("cancelling job: %w", err)
	}

	// Update metadata to reflect cancelled status
	metadata := model.RVToolsJobMetadata{
		Status: model.JobStatusCancelled,
	}
	metadataJSON, _ := json.Marshal(metadata)

	if err := s.jobStore.UpdateMetadata(ctx, jobID, metadataJSON); err != nil {
		tracer.Error(err).WithString("step", "update_metadata").Log()
	}

	tracer.Success().Log()

	// Return updated job
	return s.GetJob(ctx, jobID, orgID, username)
}
