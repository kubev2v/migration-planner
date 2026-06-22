package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AssessmentSubsetInventory interface {
	List(ctx context.Context, filter *AssessmentSubsetInventoryQueryFilter) (model.AssessmentSubsetInventoryList, error)
	Create(ctx context.Context, inventory model.AssessmentSubsetInventory) (*model.AssessmentSubsetInventory, error)
	Get(ctx context.Context, id uuid.UUID) (*model.AssessmentSubsetInventory, error)
	// Note: No Update or Delete methods - assessment subsets are immutable
	// Delete happens via CASCADE when assessment is deleted
}

type AssessmentSubsetInventoryStore struct {
	db *gorm.DB
}

// Make sure we conform to AssessmentSubsetInventory interface
var _ AssessmentSubsetInventory = (*AssessmentSubsetInventoryStore)(nil)

func NewAssessmentSubsetInventory(db *gorm.DB) AssessmentSubsetInventory {
	return &AssessmentSubsetInventoryStore{db: db}
}

func (s *AssessmentSubsetInventoryStore) List(ctx context.Context, filter *AssessmentSubsetInventoryQueryFilter) (model.AssessmentSubsetInventoryList, error) {
	var inventories model.AssessmentSubsetInventoryList
	tx := s.getDB(ctx).Model(&inventories)

	if filter != nil {
		for _, fn := range filter.QueryFn {
			tx = fn(tx)
		}
	}

	// Apply deterministic tiebreaker after filter functions
	tx = tx.Order("id")

	result := tx.Find(&inventories)
	if result.Error != nil {
		return nil, fmt.Errorf("list assessment subset inventories: %w", result.Error)
	}
	return inventories, nil
}

func (s *AssessmentSubsetInventoryStore) Create(ctx context.Context, inventory model.AssessmentSubsetInventory) (*model.AssessmentSubsetInventory, error) {
	result := s.getDB(ctx).Clauses(clause.Returning{}).Create(&inventory)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return nil, ErrDuplicateKey
		}
		return nil, fmt.Errorf("create assessment subset inventory: %w", result.Error)
	}

	return &inventory, nil
}

func (s *AssessmentSubsetInventoryStore) Get(ctx context.Context, id uuid.UUID) (*model.AssessmentSubsetInventory, error) {
	inventory := model.AssessmentSubsetInventory{ID: id}
	result := s.getDB(ctx).First(&inventory)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("get assessment subset inventory %s: %w", id, result.Error)
	}
	return &inventory, nil
}

func (s *AssessmentSubsetInventoryStore) getDB(ctx context.Context) *gorm.DB {
	tx := FromContext(ctx)
	if tx != nil {
		return tx
	}
	return s.db
}
