package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Assessment interface {
	List(ctx context.Context, filter *AssessmentQueryFilter) (model.AssessmentList, error)
	Get(ctx context.Context, id uuid.UUID) (*model.Assessment, error)
	Create(ctx context.Context, assessment model.Assessment, inventory api.Inventory) (*model.Assessment, error)
	Update(ctx context.Context, assessmentID uuid.UUID, name *string, inventory *api.Inventory) (*model.Assessment, error)
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

func (a *AssessmentStore) Create(ctx context.Context, assessment model.Assessment, inventory api.Inventory) (*model.Assessment, error) {
	// Create the assessment first
	result := a.getDB(ctx).Clauses(clause.Returning{}).Create(&assessment)
	if result.Error != nil {
		return nil, result.Error
	}

	// Create the initial snapshot with the inventory
	snapshot := model.Snapshot{
		AssessmentID: assessment.ID,
		Inventory:    model.MakeJSONField(inventory),
	}

	if err := a.getDB(ctx).Create(&snapshot).Error; err != nil {
		return nil, err
	}

	// Return the assessment with snapshots loaded
	return a.Get(ctx, assessment.ID)
}

func (a *AssessmentStore) Update(ctx context.Context, assessmentID uuid.UUID, name *string, inventory *api.Inventory) (*model.Assessment, error) {
	// Check if assessment exists
	var assessment model.Assessment
	if err := a.getDB(ctx).First(&assessment, "id = ?", assessmentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRecordNotFound
		}
		return nil, err
	}

	// Update assessment name if provided
	if name != nil {
		assessment.Name = *name
	}

	if inventory != nil {
		// Create a new snapshot
		snapshot := model.Snapshot{
			AssessmentID: assessmentID,
			Inventory:    model.MakeJSONField(*inventory),
		}

		if err := a.getDB(ctx).Create(&snapshot).Error; err != nil {
			return nil, err
		}
	}

	now := time.Now()
	assessment.UpdatedAt = &now
	if err := a.getDB(ctx).Model(&assessment).Updates(&assessment).Error; err != nil {
		return nil, err
	}

	// Return the updated assessment with snapshots
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
