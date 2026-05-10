package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kubev2v/migration-planner/internal/store/model"
)

type RVToolsFile interface {
	Create(ctx context.Context, id uuid.UUID, data []byte) error
	Get(ctx context.Context, id uuid.UUID) ([]byte, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type RVToolsFileStore struct {
	db *gorm.DB
}

var _ RVToolsFile = (*RVToolsFileStore)(nil)

func NewRVToolsFileStore(db *gorm.DB) RVToolsFile {
	return &RVToolsFileStore{db: db}
}

func (s *RVToolsFileStore) Create(ctx context.Context, id uuid.UUID, data []byte) error {
	file := model.RVToolsFile{
		ID:   id,
		Data: data,
	}
	if result := s.getDB(ctx).Create(&file); result.Error != nil {
		return fmt.Errorf("creating rvtools file: %w", result.Error)
	}
	return nil
}

func (s *RVToolsFileStore) Get(ctx context.Context, id uuid.UUID) ([]byte, error) {
	var file model.RVToolsFile
	result := s.getDB(ctx).First(&file, "id = ?", id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrRecordNotFound
		}
		return nil, fmt.Errorf("querying rvtools file: %w", result.Error)
	}
	return file.Data, nil
}

func (s *RVToolsFileStore) Delete(ctx context.Context, id uuid.UUID) error {
	result := s.getDB(ctx).Delete(&model.RVToolsFile{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("deleting rvtools file: %w", result.Error)
	}
	return nil
}

func (s *RVToolsFileStore) getDB(ctx context.Context) *gorm.DB {
	tx := FromContext(ctx)
	if tx != nil {
		return tx
	}
	return s.db
}
