package store

import (
	"context"
	"errors"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Source interface {
	List(ctx context.Context) (*api.SourceList, error)
	Create(ctx context.Context, sourceCreate api.SourceCreate) (*api.Source, error)
	DeleteAll(ctx context.Context) error
	Get(ctx context.Context, id uint) (*api.Source, error)
	Delete(ctx context.Context, id uint) error
	Update(ctx context.Context, id uint, status, statusInfo, inventory *string) (*api.Source, error)
	InitialMigration() error
}

type SourceStore struct {
	db  *gorm.DB
	log logrus.FieldLogger
}

// Make sure we conform to Source interface
var _ Source = (*SourceStore)(nil)

func NewSource(db *gorm.DB, log logrus.FieldLogger) Source {
	return &SourceStore{db: db, log: log}
}

func (s *SourceStore) InitialMigration() error {
	return s.db.AutoMigrate(&model.Source{})
}

func (s *SourceStore) List(ctx context.Context) (*api.SourceList, error) {
	var sources model.SourceList
	result := s.db.Model(&sources).Order("id").Find(&sources)
	if result.Error != nil {
		return nil, result.Error
	}
	apiSourceList := sources.ToApiResource()
	return &apiSourceList, nil
}

func (s *SourceStore) Create(ctx context.Context, sourceCreate api.SourceCreate) (*api.Source, error) {
	source := model.NewSourceFromApiCreateResource(&sourceCreate)
	result := s.db.Create(source)
	if result.Error != nil {
		return nil, result.Error
	}
	createdResource := source.ToApiResource()
	return &createdResource, nil
}

func (s *SourceStore) DeleteAll(ctx context.Context) error {
	result := s.db.Unscoped().Exec("DELETE FROM sources")
	return result.Error
}

func (s *SourceStore) Get(ctx context.Context, id uint) (*api.Source, error) {
	source := model.NewSourceFromId(id)
	result := s.db.First(&source)
	if result.Error != nil {
		return nil, result.Error
	}
	apiSource := source.ToApiResource()
	return &apiSource, nil
}

func (s *SourceStore) Delete(ctx context.Context, id uint) error {
	source := model.NewSourceFromId(id)
	result := s.db.Unscoped().Delete(&source)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		s.log.Infof("ERROR: %v", result.Error)
		return result.Error
	}
	return nil
}

func (s *SourceStore) Update(ctx context.Context, id uint, status, statusInfo, inventory *string) (*api.Source, error) {
	source := model.NewSourceFromId(id)
	selectFields := []string{}
	if status != nil {
		source.Status = *status
		selectFields = append(selectFields, "status")
	}
	if statusInfo != nil {
		source.StatusInfo = *statusInfo
		selectFields = append(selectFields, "status_info")
	}
	if inventory != nil {
		source.Inventory = *inventory
		selectFields = append(selectFields, "inventory")
	}

	result := s.db.Model(source).Clauses(clause.Returning{}).Select(selectFields).Updates(&source)
	if result.Error != nil {
		return nil, result.Error
	}

	apiSource := source.ToApiResource()
	return &apiSource, nil
}
