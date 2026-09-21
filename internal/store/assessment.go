package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kubev2v/migration-planner/internal/store/model"
)

type Assessment interface {
	List(ctx context.Context, filter *AssessmentQueryFilter) (model.AssessmentList, error)
	Get(ctx context.Context, id uuid.UUID) (*model.Assessment, error)
	Create(ctx context.Context, assessment model.Assessment) (*model.Assessment, error)
	Update(ctx context.Context, assessmentID uuid.UUID, name *string, inventories []model.AssessmentInventory) (*model.Assessment, error)
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
	tx := a.getDB(ctx).Model(&assessments).Order("created_at DESC").
		Preload("Snapshots", func(db *gorm.DB) *gorm.DB {
			return db.Order("snapshots.created_at DESC")
		}).
		Preload("Inventories", func(db *gorm.DB) *gorm.DB {
			return db.Order("assessment_inventories.created_at DESC")
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
	result := a.getDB(ctx).
		Preload("Snapshots", func(db *gorm.DB) *gorm.DB {
			return db.Order("snapshots.created_at DESC")
		}).
		Preload("Snapshots.SubsetInventories", func(db *gorm.DB) *gorm.DB {
			return db.Order("name ASC, id ASC")
		}).
		Preload("Inventories", func(db *gorm.DB) *gorm.DB {
			return db.Order("assessment_inventories.created_at DESC")
		}).
		First(&assessment, "id = ?", id)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrRecordNotFound
		}
		return nil, result.Error
	}
	return &assessment, nil
}

func (a *AssessmentStore) Create(ctx context.Context, assessment model.Assessment) (*model.Assessment, error) {
	result := a.getDB(ctx).Clauses(clause.Returning{}).Create(&assessment)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return nil, ErrDuplicateKey
		}
		return nil, result.Error
	}

	return a.Get(ctx, assessment.ID)
}

func (a *AssessmentStore) Update(ctx context.Context, assessmentID uuid.UUID, name *string, inventories []model.AssessmentInventory) (*model.Assessment, error) {
	var assessment model.Assessment
	if err := a.getDB(ctx).First(&assessment, "id = ?", assessmentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRecordNotFound
		}
		return nil, err
	}

	if name != nil {
		assessment.Name = *name
	}

	if len(inventories) > 0 {
		if err := a.getDB(ctx).Create(&inventories).Error; err != nil {
			return nil, err
		}
	}

	now := time.Now()
	assessment.UpdatedAt = &now
	if err := a.getDB(ctx).Model(&assessment).Updates(&assessment).Error; err != nil {
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
