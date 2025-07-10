package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"gorm.io/gorm"
)

type ShareToken interface {
	Create(ctx context.Context, shareToken model.ShareToken) (*model.ShareToken, error)
	GetByToken(ctx context.Context, token string) (*model.ShareToken, error)
	GetBySourceID(ctx context.Context, sourceID uuid.UUID) (*model.ShareToken, error)
	Delete(ctx context.Context, sourceID uuid.UUID) error
	GetSourceByToken(ctx context.Context, token string) (*model.Source, error)
	ListByOrgID(ctx context.Context, orgID string) ([]model.ShareToken, error)
}

type ShareTokenStore struct {
	db *gorm.DB
}

// Make sure we conform to ShareToken interface
var _ ShareToken = (*ShareTokenStore)(nil)

func NewShareTokenStore(db *gorm.DB) ShareToken {
	return &ShareTokenStore{db: db}
}

func (s *ShareTokenStore) Create(ctx context.Context, shareToken model.ShareToken) (*model.ShareToken, error) {
	result := s.getDB(ctx).Create(&shareToken)
	if result.Error != nil {
		return nil, result.Error
	}
	return &shareToken, nil
}

func (s *ShareTokenStore) GetByToken(ctx context.Context, token string) (*model.ShareToken, error) {
	var shareToken model.ShareToken
	result := s.getDB(ctx).Where("token = ?", token).First(&shareToken)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrRecordNotFound
		}
		return nil, result.Error
	}
	return &shareToken, nil
}

func (s *ShareTokenStore) GetBySourceID(ctx context.Context, sourceID uuid.UUID) (*model.ShareToken, error) {
	var shareToken model.ShareToken
	result := s.getDB(ctx).Where("source_id = ?", sourceID).First(&shareToken)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrRecordNotFound
		}
		return nil, result.Error
	}
	return &shareToken, nil
}

func (s *ShareTokenStore) Delete(ctx context.Context, sourceID uuid.UUID) error {
	result := s.getDB(ctx).Where("source_id = ?", sourceID).Delete(&model.ShareToken{})
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *ShareTokenStore) GetSourceByToken(ctx context.Context, token string) (*model.Source, error) {
	var shareToken model.ShareToken
	result := s.getDB(ctx).Preload("Source").Preload("Source.Agents").Preload("Source.ImageInfra").Preload("Source.Labels").Where("token = ?", token).First(&shareToken)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrRecordNotFound
		}
		return nil, result.Error
	}
	return &shareToken.Source, nil
}

func (s *ShareTokenStore) ListByOrgID(ctx context.Context, orgID string) ([]model.ShareToken, error) {
	var shareTokens []model.ShareToken
	result := s.getDB(ctx).Joins("JOIN sources ON share_tokens.source_id = sources.id").
		Where("sources.org_id = ?", orgID).
		Find(&shareTokens)
	if result.Error != nil {
		return nil, result.Error
	}
	return shareTokens, nil
}

func (s *ShareTokenStore) getDB(ctx context.Context) *gorm.DB {
	tx := FromContext(ctx)
	if tx != nil {
		return tx
	}
	return s.db
} 
