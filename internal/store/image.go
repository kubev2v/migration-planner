package store

import (
	"context"

	"github.com/kubev2v/migration-planner/internal/store/model"
	"gorm.io/gorm"
)

type ImageInfra interface {
	Create(ctx context.Context, imageInfra model.ImageInfra) (*model.ImageInfra, error)
}

type ImageInfraStore struct {
	db *gorm.DB
}

func NewImageInfraStore(db *gorm.DB) ImageInfra {
	return &ImageInfraStore{db: db}
}

func (i *ImageInfraStore) Create(ctx context.Context, image model.ImageInfra) (*model.ImageInfra, error) {
	if err := i.getDB(ctx).WithContext(ctx).Create(&image).Error; err != nil {
		return nil, err
	}

	return &image, nil
}

func (i *ImageInfraStore) getDB(ctx context.Context) *gorm.DB {
	tx := FromContext(ctx)
	if tx != nil {
		return tx
	}
	return i.db
}
