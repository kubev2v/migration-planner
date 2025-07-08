package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"gorm.io/gorm"
)

type Label interface {
	DeleteBySourceID(ctx context.Context, sourceID string) error
	UpdateLabels(ctx context.Context, sourceID uuid.UUID, labels []model.Label) error
}

type labelStore struct {
	db *gorm.DB
}

func NewLabelStore(db *gorm.DB) Label {
	return &labelStore{db: db}
}

func (l *labelStore) DeleteBySourceID(ctx context.Context, sourceID string) error {
	return l.getDB(ctx).WithContext(ctx).Where("source_id = ?", sourceID).Delete(&model.Label{}).Error
}

func (l *labelStore) UpdateLabels(ctx context.Context, sourceID uuid.UUID, labels []model.Label) error {
	db := l.getDB(ctx).WithContext(ctx)
	sourceIDStr := sourceID.String()

	// First delete existing labels using the DeleteBySourceID method
	if err := l.DeleteBySourceID(ctx, sourceIDStr); err != nil {
		return fmt.Errorf("failed to delete existing labels: %w", err)
	}

	if len(labels) == 0 {
		return nil
	}

	// Create new labels
	for i := range labels {
		// Ensure source ID is set correctly
		labels[i].SourceID = sourceIDStr
		if err := db.Create(&labels[i]).Error; err != nil {
			return fmt.Errorf("failed to create label with key '%s': %w", labels[i].Key, err)
		}
	}

	return nil
}

func (l *labelStore) getDB(ctx context.Context) *gorm.DB {
	tx := FromContext(ctx)
	if tx != nil {
		return tx
	}
	return l.db
}
