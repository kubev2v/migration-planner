package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Assessment interface {
	List(ctx context.Context, filter *AssessmentQueryFilter) (model.AssessmentList, error)
	Get(ctx context.Context, id uuid.UUID) (*model.Assessment, error)
	Create(ctx context.Context, assessment *model.Assessment, snapshot *model.Snapshot) error
	Update(ctx context.Context, assessmentID uuid.UUID, updates *model.Assessment, newSnapshot *model.Snapshot) (*model.Assessment, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type AssessmentStore struct {
	db *gorm.DB
}

// Make sure we conform to Assessment interface
var _ Assessment = (*AssessmentStore)(nil)

func NewAssessmentStore(db *gorm.DB) Assessment {
	return &AssessmentStore{db: db}
}

func (a *AssessmentStore) List(ctx context.Context, filter *AssessmentQueryFilter) (model.AssessmentList, error) {
	var assessments model.AssessmentList
	tx := a.getDB(ctx).Model(&assessments).Order("created_at DESC").Preload("Snapshots", func(db *gorm.DB) *gorm.DB {
		return db.Order("snapshots.created_at DESC")
	})

	if filter != nil {
		for _, fn := range filter.QueryFn {
			tx = fn(tx)
		}
	}

	result := tx.Find(&assessments)
	if result.Error != nil {
		return nil, result.Error
	}
	return assessments, nil
}

func (a *AssessmentStore) Get(ctx context.Context, id uuid.UUID) (*model.Assessment, error) {
	var assessment model.Assessment
	result := a.getDB(ctx).Preload("Snapshots", func(db *gorm.DB) *gorm.DB {
		return db.Order("snapshots.created_at DESC")
	}).First(&assessment, "id = ?", id)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrRecordNotFound
		}
		return nil, result.Error
	}
	return &assessment, nil
}

// Create creates an assessment with its associated snapshot
// Both are created in the same database operation (should be within a transaction)
func (a *AssessmentStore) Create(ctx context.Context, assessment *model.Assessment, snapshot *model.Snapshot) error {
	// Create the assessment first
	result := a.getDB(ctx).Clauses(clause.Returning{}).Create(assessment)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return ErrDuplicateKey
		}
		return result.Error
	}

	// Set the assessment ID on the snapshot
	snapshot.AssessmentID = assessment.ID

	// Create the snapshot
	if err := a.getDB(ctx).Create(snapshot).Error; err != nil {
		return err
	}

	return nil
}

func (a *AssessmentStore) Update(ctx context.Context, assessmentID uuid.UUID, updates *model.Assessment, newSnapshot *model.Snapshot) (*model.Assessment, error) {
	// Check if assessment exists
	var assessment model.Assessment
	if err := a.getDB(ctx).First(&assessment, "id = ?", assessmentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRecordNotFound
		}
		return nil, err
	}

	// Always set UpdatedAt when updating
	now := time.Now()
	updates.UpdatedAt = &now

	if err := a.getDB(ctx).Model(&assessment).Updates(updates).Error; err != nil {
		return nil, err
	}

	// Create new snapshot if provided
	if newSnapshot != nil {
		newSnapshot.AssessmentID = assessmentID
		if err := a.getDB(ctx).Create(newSnapshot).Error; err != nil {
			return nil, err
		}
	}

	// Reload to get all fields
	if err := a.getDB(ctx).First(&assessment, "id = ?", assessmentID).Error; err != nil {
		return nil, err
	}

	return &assessment, nil
}

func (a *AssessmentStore) Delete(ctx context.Context, id uuid.UUID) error {
	result := a.getDB(ctx).Unscoped().Delete(&model.Assessment{}, "id = ?", id.String())
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return result.Error
	}
	return nil
}

func (a *AssessmentStore) getDB(ctx context.Context) *gorm.DB {
	tx := FromContext(ctx)
	if tx != nil {
		return tx
	}
	return a.db
}
