package store

import (
	"context"
	"errors"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"gorm.io/gorm"
)

type Snapshot interface {
	Get(ctx context.Context, id uint) (*model.Snapshot, error)
	List(ctx context.Context, filter *SnapshotQueryFilter) ([]model.Snapshot, error)
	UpdateStatus(ctx context.Context, id uint, status model.SnapshotStatus, errorMsg *string) error
	UpdateInventory(ctx context.Context, id uint, inventory api.Inventory) error
	Delete(ctx context.Context, id uint) error
}

type SnapshotStore struct {
	db *gorm.DB
}

var _ Snapshot = (*SnapshotStore)(nil)

func NewSnapshotStore(db *gorm.DB) Snapshot {
	return &SnapshotStore{db: db}
}

func (s *SnapshotStore) Get(ctx context.Context, id uint) (*model.Snapshot, error) {
	var snapshot model.Snapshot
	result := s.getDB(ctx).First(&snapshot, id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrRecordNotFound
		}
		return nil, result.Error
	}
	return &snapshot, nil
}

func (s *SnapshotStore) List(ctx context.Context, filter *SnapshotQueryFilter) ([]model.Snapshot, error) {
	var snapshots []model.Snapshot
	tx := s.getDB(ctx).Model(&snapshots).Order("created_at DESC")

	if filter != nil {
		for _, fn := range filter.QueryFn {
			tx = fn(tx)
		}
	}

	result := tx.Find(&snapshots)
	if result.Error != nil {
		return nil, result.Error
	}
	return snapshots, nil
}

func (s *SnapshotStore) UpdateStatus(ctx context.Context, id uint, status model.SnapshotStatus, errorMsg *string) error {
	updates := model.Snapshot{
		Status: status,
		Error:  errorMsg,
	}
	result := s.getDB(ctx).Model(&model.Snapshot{}).Where(&model.Snapshot{ID: id}).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrRecordNotFound
	}
	return nil
}

func (s *SnapshotStore) UpdateInventory(ctx context.Context, id uint, inventory api.Inventory) error {
	updates := model.Snapshot{
		Inventory: model.MakeJSONField(inventory),
	}
	result := s.getDB(ctx).Model(&model.Snapshot{}).Where(&model.Snapshot{ID: id}).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrRecordNotFound
	}
	return nil
}

func (s *SnapshotStore) Delete(ctx context.Context, id uint) error {
	result := s.getDB(ctx).Delete(&model.Snapshot{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrRecordNotFound
	}
	return nil
}

func (s *SnapshotStore) getDB(ctx context.Context) *gorm.DB {
	tx := FromContext(ctx)
	if tx != nil {
		return tx
	}
	return s.db
}
