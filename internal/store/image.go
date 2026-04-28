package store

import (
	"context"

	"gorm.io/gorm"

	"github.com/kubev2v/migration-planner/internal/store/model"
)

type ImageInfra interface {
	Create(ctx context.Context, imageInfra model.ImageInfra) (*model.ImageInfra, error)
	Update(ctx context.Context, imageInfra model.ImageInfra) (*model.ImageInfra, error)
	UpdateAgentVersion(ctx context.Context, sourceID string, agentVersion string) error
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

func (i *ImageInfraStore) Update(ctx context.Context, image model.ImageInfra) (*model.ImageInfra, error) {
	// Exclude agent_version to prevent overwriting concurrent updates from OVA downloads
	if err := i.getDB(ctx).WithContext(ctx).Omit("agent_version").Save(&image).Error; err != nil {
		return nil, err
	}
	return &image, nil
}

// UpdateAgentVersion atomically updates only the agent_version field for a given source_id.
// This prevents race conditions when multiple concurrent downloads occur.
func (i *ImageInfraStore) UpdateAgentVersion(ctx context.Context, sourceID string, agentVersion string) error {
	result := i.getDB(ctx).WithContext(ctx).
		Model(&model.ImageInfra{}).
		Where("source_id = ?", sourceID).
		Update("agent_version", agentVersion)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return ErrRecordNotFound
	}

	return nil
}

func (i *ImageInfraStore) getDB(ctx context.Context) *gorm.DB {
	tx := FromContext(ctx)
	if tx != nil {
		return tx
	}
	return i.db
}
