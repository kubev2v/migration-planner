package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/rvtools/jobs"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/log"
)

// JobService handles job-related operations.
type JobService struct {
	riverClient *river.Client[pgx.Tx]
	pool        *pgxpool.Pool
	jobStore    store.Job
	logger      *log.StructuredLogger
}

// NewJobService creates a new job service.
func NewJobService(s store.Store, riverClient *river.Client[pgx.Tx], pool *pgxpool.Pool) *JobService {
	return &JobService{
		riverClient: riverClient,
		pool:        pool,
		jobStore:    s.Job(),
		logger:      log.NewDebugLogger("job_service"),
	}
}

// CreateRVToolsJob stores the file content and creates a River job atomically within a single transaction.
func (s *JobService) CreateRVToolsJob(ctx context.Context, args jobs.RVToolsJobArgs, fileContent []byte) (*v1alpha1.Job, error) {
	logger := s.logger.WithContext(ctx)
	tracer := logger.Operation("create_rvtools_job").
		WithString("name", args.Name).
		WithString("org_id", args.OrgID).
		WithString("username", args.Username).
		Build()

	fileID := uuid.New()
	args.FileID = fileID

	// Atomic: store file + insert River job in a single pgx transaction
	var insertedJob *rivertype.JobInsertResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			"INSERT INTO rvtools_files (id, data, created_at) VALUES ($1, $2, now())",
			fileID, fileContent); err != nil {
			return fmt.Errorf("storing rvtools file: %w", err)
		}

		var err error
		insertedJob, err = s.riverClient.InsertTx(ctx, tx, args, nil)
		if err != nil {
			return fmt.Errorf("inserting job: %w", err)
		}
		return nil
	})
	if err != nil {
		tracer.Error(err).Log()
		return nil, err
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

	job, err := s.GetJob(ctx, jobID, orgID, username)
	if err != nil {
		return nil, err
	}

	if job.Status == v1alpha1.JobStatusCompleted || job.Status == v1alpha1.JobStatusFailed || job.Status == v1alpha1.JobStatusCancelled {
		return job, nil
	}

	_, err = s.riverClient.JobCancel(ctx, jobID)
	if err != nil {
		tracer.Error(err).Log()
		return nil, fmt.Errorf("cancelling job: %w", err)
	}

	metadata := model.RVToolsJobMetadata{
		Status: model.JobStatusCancelled,
	}
	metadataJSON, _ := json.Marshal(metadata)

	if err := s.jobStore.UpdateMetadata(ctx, jobID, metadataJSON); err != nil {
		tracer.Error(err).WithString("step", "update_metadata").Log()
	}

	tracer.Success().Log()

	return s.GetJob(ctx, jobID, orgID, username)
}
