package service

import (
	"context"
	"encoding/base64"
	"encoding/json"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/rvtools/jobs"
	"github.com/kubev2v/migration-planner/pkg/log"
)

type JobService struct {
	client *jobs.Client
	logger *log.StructuredLogger
}

func NewJobService(client *jobs.Client) *JobService {
	return &JobService{
		client: client,
		logger: log.NewDebugLogger("job_service"),
	}
}

type JobInfo struct {
	ID     int64
	Status string
	Error  string
}

func (s *JobService) CreateRVToolsJob(ctx context.Context, fileData []byte, user *auth.User) (*JobInfo, error) {
	tracer := s.logger.WithContext(ctx).Operation("create_rvtools_job").Build()

	jobID, err := s.client.InsertJob(ctx, jobs.RVToolsArgs{
		OrgID:    user.Organization,
		Username: user.Username,
		FileData: base64.StdEncoding.EncodeToString(fileData),
	})
	if err != nil {
		tracer.Error(err).Log()
		return nil, err
	}

	tracer.Success().WithParam("job_id", jobID).Log()
	return &JobInfo{ID: jobID, Status: string(rivertype.JobStateAvailable)}, nil
}

func (s *JobService) GetJob(ctx context.Context, jobID int64, user *auth.User) (*JobInfo, error) {
	tracer := s.logger.WithContext(ctx).Operation("get_job").WithParam("job_id", jobID).Build()

	row, err := s.client.JobGet(ctx, jobID)
	if err != nil {
		if err == river.ErrNotFound {
			return nil, NewErrJobNotFound(jobID)
		}
		tracer.Error(err).Log()
		return nil, err
	}

	if err := s.checkAccess(row, user); err != nil {
		return nil, err
	}

	tracer.Success().Log()
	return rowToJobInfo(row), nil
}

func (s *JobService) CancelJob(ctx context.Context, jobID int64, user *auth.User) (*JobInfo, error) {
	tracer := s.logger.WithContext(ctx).Operation("cancel_job").WithParam("job_id", jobID).Build()

	row, err := s.client.JobGet(ctx, jobID)
	if err != nil {
		if err == river.ErrNotFound {
			return nil, NewErrJobNotFound(jobID)
		}
		tracer.Error(err).Log()
		return nil, err
	}

	if err := s.checkAccess(row, user); err != nil {
		return nil, err
	}

	if isJobFinished(row.State) {
		return nil, NewErrJobAlreadyCompleted(jobID)
	}

	cancelled, err := s.client.JobCancel(ctx, jobID)
	if err != nil {
		tracer.Error(err).Log()
		return nil, err
	}

	tracer.Success().Log()
	return rowToJobInfo(cancelled), nil
}

func (s *JobService) GetJobInventory(ctx context.Context, jobID int64, user *auth.User) ([]byte, error) {
	tracer := s.logger.WithContext(ctx).Operation("get_job_inventory").WithParam("job_id", jobID).Build()

	row, err := s.client.JobGet(ctx, jobID)
	if err != nil {
		if err == river.ErrNotFound {
			return nil, NewErrJobNotFound(jobID)
		}
		tracer.Error(err).Log()
		return nil, err
	}

	if err := s.checkAccess(row, user); err != nil {
		return nil, err
	}

	if row.State != rivertype.JobStateCompleted {
		return nil, NewErrJobNotCompleted(jobID)
	}

	output := row.Output()
	if output == nil {
		return nil, NewErrJobNotCompleted(jobID)
	}

	tracer.Success().Log()
	return output, nil
}

func (s *JobService) checkAccess(row *rivertype.JobRow, user *auth.User) error {
	var args jobs.RVToolsArgs
	if err := json.Unmarshal(row.EncodedArgs, &args); err != nil {
		return err
	}
	if args.Username != user.Username || args.OrgID != user.Organization {
		return NewErrJobAccessForbidden(row.ID)
	}
	return nil
}

func rowToJobInfo(row *rivertype.JobRow) *JobInfo {
	info := &JobInfo{ID: row.ID, Status: string(row.State)}
	if len(row.Errors) > 0 {
		info.Error = row.Errors[len(row.Errors)-1].Error
	}
	return info
}

func isJobFinished(state rivertype.JobState) bool {
	switch state {
	case rivertype.JobStateCompleted, rivertype.JobStateCancelled, rivertype.JobStateDiscarded:
		return true
	default:
		return false
	}
}
