package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kubev2v/migration-planner/internal/model"
	"github.com/kubev2v/migration-planner/internal/store/mappers"
	storemodel "github.com/kubev2v/migration-planner/internal/store/model"
)

type Assessment interface {
	List(ctx context.Context, filter *AssessmentQueryFilter) (model.AssessmentList, error)
	Get(ctx context.Context, id uuid.UUID) (*model.Assessment, error)
	Create(ctx context.Context, assessment model.Assessment) (*model.Assessment, error)
	Update(ctx context.Context, assessmentID uuid.UUID, name *string, inventories []model.AssessmentInventory) (*model.Assessment, error)
	UpsertInventories(ctx context.Context, inventory ...model.AssessmentInventory) error
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
	var assessments storemodel.AssessmentList
	tx := a.getDB(ctx).Model(&assessments).Order("created_at DESC")

	if filter != nil {
		for _, fn := range filter.QueryFn {
			tx = fn(tx)
		}
	}

	result := tx.Find(&assessments)
	if result.Error != nil {
		return nil, result.Error
	}
	return mappers.AssessmentSummaryListFromStore(assessments), nil
}

func (a *AssessmentStore) Get(ctx context.Context, id uuid.UUID) (*model.Assessment, error) {
	var assessment storemodel.Assessment
	result := a.getDB(ctx).
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
	domainAssessment, err := mappers.AssessmentFromStore(&assessment)
	if err != nil {
		return nil, fmt.Errorf("failed to convert assessment: %w", err)
	}
	return &domainAssessment, nil
}

func (a *AssessmentStore) Create(ctx context.Context, assessment model.Assessment) (*model.Assessment, error) {
	storedAssessment, err := mappers.AssessmentToStore(assessment)
	if err != nil {
		return nil, fmt.Errorf("failed to convert assessment for storage: %w", err)
	}
	result := a.getDB(ctx).Clauses(clause.Returning{}).Create(&storedAssessment)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return nil, ErrDuplicateKey
		}
		return nil, result.Error
	}

	return a.Get(ctx, assessment.ID)
}

func (a *AssessmentStore) Update(ctx context.Context, assessmentID uuid.UUID, name *string, inventories []model.AssessmentInventory) (*model.Assessment, error) {
	var assessment storemodel.Assessment
	if err := a.getDB(ctx).First(&assessment, "id = ?", assessmentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRecordNotFound
		}
		return nil, err
	}

	if name != nil {
		assessment.Name = *name
	}

	stored, err := mappers.AssessmentInventoriesToStore(inventories...)
	if err != nil {
		return nil, fmt.Errorf("failed to convert inventories to store: %w", err)
	}

	if len(stored) > 0 {
		if err := a.getDB(ctx).Create(&stored).Error; err != nil {
			return nil, err
		}
	}

	now := time.Now()
	assessment.UpdatedAt = &now
	if err := a.getDB(ctx).Model(&assessment).Updates(&assessment).Error; err != nil {
		return nil, err
	}

	domainAssessment, err := mappers.AssessmentFromStore(&assessment)
	if err != nil {
		return nil, fmt.Errorf("failed to convert assessment: %w", err)
	}
	return &domainAssessment, nil
}

func (a *AssessmentStore) UpsertInventories(ctx context.Context, inventories ...model.AssessmentInventory) error {
	stored, err := mappers.AssessmentInventoriesToStore(inventories...)
	if err != nil {
		return fmt.Errorf("failed to convert inventories to store: %w", err)
	}

	if err := a.getDB(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&stored).Error; err != nil {
		return err
	}

	return nil
}

func (a *AssessmentStore) Delete(ctx context.Context, id uuid.UUID) error {
	result := a.getDB(ctx).Unscoped().Delete(&storemodel.Assessment{}, "id = ?", id.String())
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
