package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrRecordNotFound error = errors.New("record not found")
)

type Source interface {
	List(ctx context.Context) (api.SourceList, error)
	Create(ctx context.Context, id uuid.UUID) (*api.Source, error)
	DeleteAll(ctx context.Context) error
	Get(ctx context.Context, id uuid.UUID) (*api.Source, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Update(ctx context.Context, id uuid.UUID, inventory *api.Inventory) (*api.Source, error)
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

func (s *SourceStore) List(ctx context.Context) (api.SourceList, error) {
	var sources model.SourceList
	result := s.getDB(ctx).Model(&sources).Order("id").Find(&sources)
	if result.Error != nil {
		return nil, result.Error
	}
	return sources.ToApiResource(), nil
}

func (s *SourceStore) Create(ctx context.Context, id uuid.UUID) (*api.Source, error) {
	source := model.NewSourceFromApiCreateResource(id)
	result := s.getDB(ctx).Create(source)
	if result.Error != nil {
		return nil, result.Error
	}
	createdResource := source.ToApiResource()
	return &createdResource, nil
}

func (s *SourceStore) DeleteAll(ctx context.Context) error {
	result := s.getDB(ctx).Unscoped().Exec("DELETE FROM sources")
	return result.Error
}

func (s *SourceStore) Get(ctx context.Context, id uuid.UUID) (*api.Source, error) {
	source := model.NewSourceFromId(id)
	result := s.getDB(ctx).Preload("Agents").First(&source)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrRecordNotFound
		}
		return nil, result.Error
	}
	apiSource := source.ToApiResource()
	return &apiSource, nil
}

func (s *SourceStore) Delete(ctx context.Context, id uuid.UUID) error {
	source := model.NewSourceFromId(id)
	result := s.getDB(ctx).Unscoped().Delete(&source)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		zap.S().Named("source_store").Infof("ERROR: %v", result.Error)
		return result.Error
	}
	return nil
}

func (s *SourceStore) Update(ctx context.Context, id uuid.UUID, inventory *api.Inventory) (*api.Source, error) {
	source := model.NewSourceFromId(id)
	selectFields := []string{}
	if inventory != nil {
		source.Inventory = model.MakeJSONField(*inventory)
		selectFields = append(selectFields, "inventory")
	}
	result := s.getDB(ctx).Model(source).Clauses(clause.Returning{}).Select(selectFields).Updates(&source)
	if result.Error != nil {
		return nil, result.Error
	}

	apiSource := source.ToApiResource()
	return &apiSource, nil
}

func (s *SourceStore) getDB(ctx context.Context) *gorm.DB {
	tx := FromContext(ctx)
	if tx != nil {
		return tx
	}
	return s.db
}
