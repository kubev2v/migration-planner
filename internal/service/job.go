package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/log"
	"github.com/riverqueue/river"
)

type RVToolsJobArgs struct {
	Name      string `json:"name"`
	FilePath  string `json:"file_path"`
	OrgID     string `json:"org_id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

func (RVToolsJobArgs) Kind() string {
	return "rvtools_assessment"
}

func (RVToolsJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       "default",
		MaxAttempts: 1,
	}
}

type JobService struct {
	riverClient *river.Client[pgx.Tx]
	jobStore    store.Job
	queue       string
	logger      *log.StructuredLogger
}

func NewJobService(s store.Store, riverClient *river.Client[pgx.Tx], queue string) *JobService {
	return &JobService{
		riverClient: riverClient,
		jobStore:    s.Job(),
		queue:       queue,
		logger:      log.NewDebugLogger("job_service"),
	}
}

func (s *JobService) CreateRVToolsJob(ctx context.Context, args RVToolsJobArgs) (*v1alpha1.Job, error) {
	logger := s.logger.WithContext(ctx)
	tracer := logger.Operation("create_rvtools_job").
		WithString("name", args.Name).
		WithString("org_id", args.OrgID).
		WithString("username", args.Username).
		Build()

	insertedJob, err := s.riverClient.Insert(ctx, args, &river.InsertOpts{Queue: s.queue, MaxAttempts: 1})
	if err != nil {
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

func (s *JobService) GetJob(ctx context.Context, jobID int64, orgID, username string) (*v1alpha1.Job, error) {
	logger := s.logger.WithContext(ctx)
	tracer := logger.Operation("get_job").
		WithParam("job_id", jobID).
		Build()

	jobRow, err := s.jobStore.Get(ctx, jobID)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return nil, NewErrJobNotFound(jobID)
		}
		tracer.Error(err).Log()
		return nil, fmt.Errorf("querying job: %w", err)
	}

	var args RVToolsJobArgs
	if err := json.Unmarshal(jobRow.ArgsJSON, &args); err != nil {
		tracer.Error(err).Log()
		return nil, fmt.Errorf("parsing job args: %w", err)
	}

	if args.OrgID != orgID || args.Username != username {
		return nil, NewErrJobForbidden(jobID)
	}

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
