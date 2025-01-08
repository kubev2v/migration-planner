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

var (
	ErrRecordNotFound error = errors.New("record not found")
)

type Source interface {
	List(ctx context.Context, filter *SourceQueryFilter) (model.SourceList, error)
	Create(ctx context.Context, source model.Source) (*model.Source, error)
	DeleteAll(ctx context.Context) error
	Get(ctx context.Context, id uuid.UUID) (*model.Source, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Update(ctx context.Context, source model.Source) (*model.Source, error)
	InitialMigration(context.Context) error
}

type SourceStore struct {
	db *gorm.DB
}

// Make sure we conform to Source interface
var _ Source = (*SourceStore)(nil)

func NewSource(db *gorm.DB) Source {
	return &SourceStore{db: db}
}

func (s *SourceStore) InitialMigration(ctx context.Context) error {
	return s.getDB(ctx).AutoMigrate(&model.Source{})
}

func (s *SourceStore) List(ctx context.Context, filter *SourceQueryFilter) (model.SourceList, error) {
	var sources model.SourceList
	tx := s.getDB(ctx).Model(&sources).Order("id").Preload("Agents")

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
	result := s.getDB(ctx).Create(&source)
	if result.Error != nil {
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
	result := s.getDB(ctx).Preload("Agents").First(&source)
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
	selectFields := []string{}
	if source.Inventory != nil {
		selectFields = append(selectFields, "inventory")
	}
	result := s.getDB(ctx).Model(&source).Clauses(clause.Returning{}).Select(selectFields).Updates(&source)
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
