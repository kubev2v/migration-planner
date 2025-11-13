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
	Create(ctx context.Context, assessment model.Assessment, inventory *api.Inventory) (*model.Assessment, error)
	UpdateName(ctx context.Context, assessmentID uuid.UUID, name string) (*model.Assessment, error)
	AddSnapshot(ctx context.Context, assessmentID uuid.UUID, inventory *api.Inventory) error
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

// Create creates an assessment with a snapshot
// If inventory is nil, creates a pending snapshot for async processing
// If inventory is provided, creates a ready snapshot with the inventory
func (a *AssessmentStore) Create(ctx context.Context, assessment model.Assessment, inventory *api.Inventory) (*model.Assessment, error) {
	db := a.getDB(ctx)

	// Wrap both creates in a transaction to prevent dangling assessment
	err := db.Transaction(func(tx *gorm.DB) error {
		// Create the assessment first
		result := tx.Clauses(clause.Returning{}).Create(&assessment)
		if result.Error != nil {
			if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
				return ErrDuplicateKey
			}
			return result.Error
		}

		// Create snapshot - status depends on whether inventory is provided
		snapshot := a.buildInitialSnapshot(assessment.ID, inventory)
		if err := tx.Create(&snapshot).Error; err != nil {
			return err
		}

		// Populate the Snapshots relationship on the assessment
		assessment.Snapshots = []model.Snapshot{snapshot}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return &assessment, nil
}

// buildInitialSnapshot creates the initial snapshot based on whether inventory is provided
func (a *AssessmentStore) buildInitialSnapshot(assessmentID uuid.UUID, inventory *api.Inventory) model.Snapshot {
	snapshot := model.Snapshot{
		AssessmentID: assessmentID,
	}

	if inventory != nil {
		// Sync flow: with inventory, status = ready
		snapshot.Status = model.SnapshotStatusReady
		snapshot.Inventory = model.MakeJSONField(*inventory)
	} else {
		// Async flow: no inventory, status = pending
		snapshot.Status = model.SnapshotStatusPending
	}

	return snapshot
}

func (a *AssessmentStore) UpdateName(ctx context.Context, assessmentID uuid.UUID, name string) (*model.Assessment, error) {
	var assessment model.Assessment
	if err := a.getDB(ctx).First(&assessment, "id = ?", assessmentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRecordNotFound
		}
		return nil, err
	}

	assessment.Name = name
	now := time.Now()
	assessment.UpdatedAt = &now

	if err := a.getDB(ctx).Model(&assessment).Updates(&assessment).Error; err != nil {
		return nil, err
	}

	return &assessment, nil
}

func (a *AssessmentStore) AddSnapshot(ctx context.Context, assessmentID uuid.UUID, inventory *api.Inventory) error {
	// Check if assessment exists
	var assessment model.Assessment
	if err := a.getDB(ctx).First(&assessment, "id = ?", assessmentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrRecordNotFound
		}
		return err
	}

	// Create a new snapshot
	snapshot := a.buildInitialSnapshot(assessmentID, inventory)
	if err := a.getDB(ctx).Create(&snapshot).Error; err != nil {
		return err
	}

	// Update assessment's updated_at timestamp
	now := time.Now()
	assessment.UpdatedAt = &now
	if err := a.getDB(ctx).Model(&assessment).Updates(&assessment).Error; err != nil {
		return err
	}

	return nil
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
