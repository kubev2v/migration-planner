package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ClusterSizingInput interface {
	// Upsert inserts or replaces cluster sizing input.
	// Uses "replace" semantics: nil fields clear database columns.
	Upsert(ctx context.Context, input model.AssessmentClusterSizingInput) (*model.AssessmentClusterSizingInput, error)

	// Get retrieves cluster sizing input. Returns ErrRecordNotFound if not found.
	Get(ctx context.Context, assessmentID uuid.UUID, clusterID string) (*model.AssessmentClusterSizingInput, error)
}

type ClusterSizingInputStore struct {
	db *gorm.DB
}

var _ ClusterSizingInput = (*ClusterSizingInputStore)(nil)

func NewClusterSizingInputStore(db *gorm.DB) ClusterSizingInput {
	return &ClusterSizingInputStore{db: db}
}

func (s *ClusterSizingInputStore) Upsert(ctx context.Context, input model.AssessmentClusterSizingInput) (*model.AssessmentClusterSizingInput, error) {
	now := time.Now()
	input.UpdatedAt = &now

	result := s.getDB(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "assessment_id"},
			{Name: "external_cluster_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"cpu_over_commit_ratio",
			"memory_over_commit_ratio",
			"worker_node_cpu",
			"worker_node_threads",
			"worker_node_memory",
			"control_plane_schedulable",
			"control_plane_node_count",
			"control_plane_cpu",
			"control_plane_memory",
			"hosted_control_plane",
			"updated_at",
		}),
	}).Create(&input)
	if result.Error != nil {
		return nil, fmt.Errorf("upserting cluster sizing input: %w", result.Error)
	}

	return &input, nil
}

func (s *ClusterSizingInputStore) Get(ctx context.Context, assessmentID uuid.UUID, clusterID string) (*model.AssessmentClusterSizingInput, error) {
	var input model.AssessmentClusterSizingInput
	result := s.getDB(ctx).First(&input, "assessment_id = ? AND external_cluster_id = ?", assessmentID, clusterID)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("querying cluster sizing input: %w", result.Error)
	}

	return &input, nil
}

func (s *ClusterSizingInputStore) getDB(ctx context.Context) *gorm.DB {
	tx := FromContext(ctx)
	if tx != nil {
		return tx
	}
	return s.db
}
