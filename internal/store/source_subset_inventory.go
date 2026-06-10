package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SourceSubsetInventory interface {
	List(ctx context.Context, filter *SourceSubsetInventoryQueryFilter) (model.SourceSubsetInventoryList, error)
	Create(ctx context.Context, sourceInventory model.SourceSubsetInventory) (*model.SourceSubsetInventory, error)
	Get(ctx context.Context, id uuid.UUID) (*model.SourceSubsetInventory, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Update(ctx context.Context, sourceInventory model.SourceSubsetInventory) (*model.SourceSubsetInventory, error)
}

type SourceSubsetInventoryStore struct {
	db *gorm.DB
}

// Make sure we conform to SourceSubsetInventory interface
var _ SourceSubsetInventory = (*SourceSubsetInventoryStore)(nil)

func NewSourceSubsetInventory(db *gorm.DB) SourceSubsetInventory {
	return &SourceSubsetInventoryStore{db: db}
}

func (s *SourceSubsetInventoryStore) List(ctx context.Context, filter *SourceSubsetInventoryQueryFilter) (model.SourceSubsetInventoryList, error) {
	var sourceInventories model.SourceSubsetInventoryList
	tx := s.getDB(ctx).Model(&sourceInventories).Order("id")

	if filter != nil {
		for _, fn := range filter.QueryFn {
			tx = fn(tx)
		}
	}

	result := tx.Find(&sourceInventories)
	if result.Error != nil {
		return nil, result.Error
	}
	return sourceInventories, nil
}

func (s *SourceSubsetInventoryStore) Create(ctx context.Context, sourceInventory model.SourceSubsetInventory) (*model.SourceSubsetInventory, error) {
	result := s.getDB(ctx).Clauses(clause.Returning{}).Create(&sourceInventory)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return nil, ErrDuplicateKey
		}

		return nil, result.Error
	}

	return &sourceInventory, nil
}

func (s *SourceSubsetInventoryStore) Get(ctx context.Context, id uuid.UUID) (*model.SourceSubsetInventory, error) {
	sourceInventory := model.SourceSubsetInventory{ID: id}
	result := s.getDB(ctx).First(&sourceInventory)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrRecordNotFound
		}
		return nil, result.Error
	}
	return &sourceInventory, nil
}

func (s *SourceSubsetInventoryStore) Delete(ctx context.Context, id uuid.UUID) error {
	sourceInventory := model.SourceSubsetInventory{ID: id}
	result := s.getDB(ctx).Unscoped().Delete(&sourceInventory)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		zap.S().Named("source_subset_inventory_store").Infof("ERROR: %v", result.Error)
		return result.Error
	}
	return nil
}

func (s *SourceSubsetInventoryStore) Update(ctx context.Context, sourceInventory model.SourceSubsetInventory) (*model.SourceSubsetInventory, error) {
	// Use Select to explicitly update all fields, including zero-value fields like VMsCount=0 or VCenterID=""
	// Without Select, GORM's Updates() skips zero-value fields, causing stale data
	result := s.getDB(ctx).Model(&sourceInventory).
		Select("name", "source_id", "v_center_id", "vms_count", "inventory", "update_type", "updated_at").
		Clauses(clause.Returning{}).
		Updates(&sourceInventory)
	if result.Error != nil {
		return nil, result.Error
	}

	if result.RowsAffected == 0 {
		return nil, ErrRecordNotFound
	}

	return &sourceInventory, nil
}

func (s *SourceSubsetInventoryStore) getDB(ctx context.Context) *gorm.DB {
	tx := FromContext(ctx)
	if tx != nil {
		return tx
	}
	return s.db
}
