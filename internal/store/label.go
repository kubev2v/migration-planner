package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/sirupsen/logrus"
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
	// Use raw SQL to ensure the delete works correctly with composite primary keys
	return l.getDB(ctx).WithContext(ctx).Exec("DELETE FROM labels WHERE source_id = ?", sourceID).Error
}

func (l *labelStore) UpdateLabels(ctx context.Context, sourceID uuid.UUID, labels []model.Label) error {
	db := l.getDB(ctx).WithContext(ctx)
	sourceIDStr := sourceID.String()

	// First delete existing labels using the DeleteBySourceID method
	if err := l.DeleteBySourceID(ctx, sourceIDStr); err != nil {
		logrus.Errorf("Failed to delete existing labels for source ID: %s, error: %v", sourceIDStr, err)
		return err
	}

	// Create new labels
	for _, label := range labels {
		label.SourceID = sourceIDStr
		if err := db.Create(&label).Error; err != nil {
			return err
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
