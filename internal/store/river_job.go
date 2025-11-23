package store

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	RiverJobStateAvailable = "available"
	RiverJobStateRunning   = "running"
	RiverJobStateRetryable = "retryable"
	RiverJobStateScheduled = "scheduled"
)

type RiverJob interface {
	GetJob(ctx context.Context, assessmentID uuid.UUID) (*int64, error)
}

type RiverJobStore struct {
	db *gorm.DB
}

var _ RiverJob = (*RiverJobStore)(nil)

func NewRiverJobStore(db *gorm.DB) RiverJob {
	return &RiverJobStore{db: db}
}

// GetJob finds a River job ID by assessmentID in the job args
// Returns nil if no active job is found for the assessment
func (r *RiverJobStore) GetJob(ctx context.Context, assessmentID uuid.UUID) (*int64, error) {
	var jobID int64

	err := r.getDB(ctx).
		Table("river_job").
		Select("id").
		Where("state IN ?", []string{
			RiverJobStateAvailable,
			RiverJobStateRunning,
			RiverJobStateRetryable,
			RiverJobStateScheduled,
		}).
		Where("args->>'assessmentId' = ?", assessmentID.String()).
		Order("id DESC").
		Limit(1).
		Scan(&jobID).Error

	if err != nil {
		return nil, err
	}

	if jobID == 0 {
		return nil, nil
	}

	return &jobID, nil
}

func (r *RiverJobStore) getDB(ctx context.Context) *gorm.DB {
	tx := FromContext(ctx)
	if tx != nil {
		return tx
	}
	return r.db
}
