package store

import (
	"context"

	"github.com/kubev2v/migration-planner/internal/model"
	"github.com/kubev2v/migration-planner/internal/store/mappers"
	storemodel "github.com/kubev2v/migration-planner/internal/store/model"
	"gorm.io/gorm"
)

type ImageInfra interface {
	Create(ctx context.Context, imageInfra model.ImageInfra) (*model.ImageInfra, error)
	Update(ctx context.Context, imageInfra model.ImageInfra) (*model.ImageInfra, error)
	UpdateAgentVersion(ctx context.Context, sourceID string, agentVersion string) error
	UpdateAgentToken(ctx context.Context, sourceID string, agentToken string) error
}

type ImageInfraStore struct {
	db *gorm.DB
}

func NewImageInfraStore(db *gorm.DB) ImageInfra {
	return &ImageInfraStore{db: db}
}

func (i *ImageInfraStore) Create(ctx context.Context, image model.ImageInfra) (*model.ImageInfra, error) {
	stored := mappers.ImageInfraToStore(image)
	if err := i.getDB(ctx).WithContext(ctx).Create(&stored).Error; err != nil {
		return nil, err
	}

	mapped := mappers.ImageInfraFromStore(stored)
	return &mapped, nil
}

func (i *ImageInfraStore) Update(ctx context.Context, image model.ImageInfra) (*model.ImageInfra, error) {
	stored := mappers.ImageInfraToStore(image)
	// Exclude agent_version and agent_token to prevent overwriting concurrent updates
	if err := i.getDB(ctx).WithContext(ctx).Omit("agent_version", "agent_token").Save(&stored).Error; err != nil {
		return nil, err
	}
	mapped := mappers.ImageInfraFromStore(stored)
	return &mapped, nil
}

// UpdateAgentVersion atomically updates only the agent_version field for a given source_id.
// This prevents race conditions when multiple concurrent downloads occur.
func (i *ImageInfraStore) UpdateAgentVersion(ctx context.Context, sourceID string, agentVersion string) error {
	result := i.getDB(ctx).WithContext(ctx).
		Model(&storemodel.ImageInfra{}).
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

// UpdateAgentToken atomically updates only the agent_token field for a given source_id.
func (i *ImageInfraStore) UpdateAgentToken(ctx context.Context, sourceID string, agentToken string) error {
	result := i.getDB(ctx).WithContext(ctx).
		Model(&storemodel.ImageInfra{}).
		Where("source_id = ?", sourceID).
		Update("agent_token", agentToken)

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
