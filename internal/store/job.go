package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/riverqueue/river/rivertype"
	"gorm.io/gorm"
)

// JobRow represents a row from the river_job table
type JobRow struct {
	ID           int64              `gorm:"column:id;primaryKey"`
	State        rivertype.JobState `gorm:"column:state"`
	ArgsJSON     []byte             `gorm:"column:args"`
	MetadataJSON []byte             `gorm:"column:metadata"`
}

// TableName specifies the table name for GORM
func (JobRow) TableName() string {
	return "river_job"
}

// Job interface for job-related database operations
type Job interface {
	Get(ctx context.Context, id int64) (*JobRow, error)
	UpdateMetadata(ctx context.Context, id int64, metadataJSON []byte) error
}

// JobStore implements the Job interface
type JobStore struct {
	db *gorm.DB
}

// Make sure we conform to Job interface
var _ Job = (*JobStore)(nil)

// NewJobStore creates a new job store
func NewJobStore(db *gorm.DB) Job {
	return &JobStore{db: db}
}

// Get retrieves a job by ID from the river_job table
func (s *JobStore) Get(ctx context.Context, id int64) (*JobRow, error) {
	var jobRow JobRow
	result := s.getDB(ctx).First(&jobRow, "id = ?", id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("querying job: %w", result.Error)
	}

	return &jobRow, nil
}

// UpdateMetadata updates the metadata of a job
func (s *JobStore) UpdateMetadata(ctx context.Context, id int64, metadataJSON []byte) error {
	result := s.getDB(ctx).Model(&JobRow{}).Where("id = ?", id).Update("metadata", metadataJSON)
	if result.Error != nil {
		return fmt.Errorf("updating job metadata: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrRecordNotFound
	}

	return nil
}

func (s *JobStore) getDB(ctx context.Context) *gorm.DB {
	tx := FromContext(ctx)
	if tx != nil {
		return tx
	}
	return s.db
}
