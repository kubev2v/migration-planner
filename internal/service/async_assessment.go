package service

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"go.uber.org/zap"
)

// AsyncJobStatus represents the status of an asynchronous job
type AsyncJobStatus string

const (
	AsyncJobStatusPending   AsyncJobStatus = "pending"
	AsyncJobStatusRunning   AsyncJobStatus = "running"
	AsyncJobStatusCompleted AsyncJobStatus = "completed"
	AsyncJobStatusFailed    AsyncJobStatus = "failed"
)

// AsyncJob represents an asynchronous RVTools assessment creation job
type AsyncJob struct {
	ID           uuid.UUID      `json:"id"`
	Status       AsyncJobStatus `json:"status"`
	Error        string         `json:"error,omitempty"`
	AssessmentID *uuid.UUID     `json:"assessment_id,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}

// AsyncAssessmentService handles asynchronous RVTools assessment processing
type AsyncAssessmentService struct {
	assessmentService *AssessmentService
	jobs              map[uuid.UUID]*AsyncJob
	mu                sync.RWMutex
}

// NewAsyncAssessmentService creates a new async assessment service
func NewAsyncAssessmentService(assessmentService *AssessmentService) *AsyncAssessmentService {
	return &AsyncAssessmentService{
		assessmentService: assessmentService,
		jobs:              make(map[uuid.UUID]*AsyncJob),
	}
}

// CreateJob creates a new async job and returns its ID
func (as *AsyncAssessmentService) CreateJob() *AsyncJob {
	as.mu.Lock()
	defer as.mu.Unlock()

	job := &AsyncJob{
		ID:        uuid.New(),
		Status:    AsyncJobStatusPending,
		CreatedAt: time.Now(),
	}
	as.jobs[job.ID] = job
	return job
}

// GetJob retrieves a job by ID
func (as *AsyncAssessmentService) GetJob(id uuid.UUID) *AsyncJob {
	as.mu.RLock()
	defer as.mu.RUnlock()
	return as.jobs[id]
}

// updateJob updates job status internally
func (as *AsyncAssessmentService) updateJob(id uuid.UUID, status AsyncJobStatus, assessmentID *uuid.UUID, err error) {
	as.mu.Lock()
	defer as.mu.Unlock()

	job := as.jobs[id]
	if job == nil {
		return
	}

	job.Status = status
	job.AssessmentID = assessmentID
	if err != nil {
		job.Error = err.Error()
	}
}

// ProcessAssessmentAsync processes RVTools assessment creation in background
func (as *AsyncAssessmentService) ProcessAssessmentAsync(ctx context.Context, jobID uuid.UUID, createForm mappers.AssessmentCreateForm) {
	go func() {
		// Create a new context not tied to the HTTP request lifecycle
		bgCtx := context.Background()
		logger := zap.S().Named("async_assessment")

		as.updateJob(jobID, AsyncJobStatusRunning, nil, nil)
		logger.Infow("async_job_started", "job_id", jobID.String(), "assessment_name", createForm.Name)

		assessment, err := as.assessmentService.CreateAssessment(bgCtx, createForm)
		if err != nil {
			logger.Errorw("async_job_failed", "job_id", jobID.String(), "error", err.Error())
			as.updateJob(jobID, AsyncJobStatusFailed, nil, err)
			return
		}

		as.updateJob(jobID, AsyncJobStatusCompleted, &assessment.ID, nil)
		logger.Infow("async_job_completed", "job_id", jobID.String(), "assessment_id", assessment.ID.String())
	}()
}
