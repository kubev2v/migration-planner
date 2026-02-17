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

type Source interface {
	List(ctx context.Context, filter *SourceQueryFilter) (model.SourceList, error)
	Create(ctx context.Context, source model.Source) (*model.Source, error)
	DeleteAll(ctx context.Context) error
	Get(ctx context.Context, id uuid.UUID) (*model.Source, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Update(ctx context.Context, source model.Source) (*model.Source, error)
}

type SourceStore struct {
	db *gorm.DB
}

// Make sure we conform to Source interface
var _ Source = (*SourceStore)(nil)

func NewSource(db *gorm.DB) Source {
	return &SourceStore{db: db}
}

func (s *SourceStore) List(ctx context.Context, filter *SourceQueryFilter) (model.SourceList, error) {
	var sources model.SourceList
	tx := s.getDB(ctx).Model(&sources).Order("id").Preload("Agents").Preload("ImageInfra").Preload("Labels")

	if filter != nil {
		for _, fn := range filter.QueryFn {
			tx = fn(tx)
		}
	}

	result := tx.Find(&sources)
	if result.Error != nil {
		return nil, result.Error
	}
	return sources, nil
}

func (s *SourceStore) Create(ctx context.Context, source model.Source) (*model.Source, error) {
	result := s.getDB(ctx).Clauses(clause.Returning{}).Create(&source)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return nil, ErrDuplicateKey
		}

		return nil, result.Error
	}

	return &source, nil
}

func (s *SourceStore) DeleteAll(ctx context.Context) error {
	result := s.getDB(ctx).Unscoped().Exec("DELETE FROM sources")
	return result.Error
}

func (s *SourceStore) Get(ctx context.Context, id uuid.UUID) (*model.Source, error) {
	source := model.Source{ID: id}
	result := s.getDB(ctx).Preload("Agents").Preload("ImageInfra").Preload("Labels").First(&source)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrRecordNotFound
		}
		return nil, result.Error
	}
	return &source, nil
}

func (s *SourceStore) Delete(ctx context.Context, id uuid.UUID) error {
	source := model.Source{ID: id}
	result := s.getDB(ctx).Unscoped().Delete(&source)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		zap.S().Named("source_store").Infof("ERROR: %v", result.Error)
		return result.Error
	}
	return nil
}

func (s *SourceStore) Update(ctx context.Context, source model.Source) (*model.Source, error) {
	result := s.getDB(ctx).Model(&source).Clauses(clause.Returning{}).Updates(&source)
	if result.Error != nil {
		return nil, result.Error
	}

	return &source, nil
}

func (s *SourceStore) getDB(ctx context.Context) *gorm.DB {
	tx := FromContext(ctx)
	if tx != nil {
		return tx
	}
	return s.db
}
