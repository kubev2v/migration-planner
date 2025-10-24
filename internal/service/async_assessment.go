package service

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
)

// AsyncJobStatus represents the status of an asynchronous job
type AsyncJobStatus string

const (
	AsyncJobStatusPending   AsyncJobStatus = "pending"
	AsyncJobStatusRunning   AsyncJobStatus = "running"
	AsyncJobStatusCompleted AsyncJobStatus = "completed"
	AsyncJobStatusFailed    AsyncJobStatus = "failed"
)

// AsyncJob represents an asynchronous assessment creation job
type AsyncJob struct {
	ID           uuid.UUID      `json:"id"`
	Status       AsyncJobStatus `json:"status"`
	Error        string         `json:"error,omitempty"`
	AssessmentID *uuid.UUID     `json:"assessment_id,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}

// AsyncAssessmentService handles asynchronous assessment processing
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

// ProcessAssessmentAsync processes assessment creation in background
func (as *AsyncAssessmentService) ProcessAssessmentAsync(ctx context.Context, jobID uuid.UUID, createForm mappers.AssessmentCreateForm) {
	go func() {
		as.updateJob(jobID, AsyncJobStatusRunning, nil, nil)

		assessment, err := as.assessmentService.CreateAssessment(ctx, createForm)
		if err != nil {
			as.updateJob(jobID, AsyncJobStatusFailed, nil, err)
			return
		}

		as.updateJob(jobID, AsyncJobStatusCompleted, &assessment.ID, nil)
	}()
}
