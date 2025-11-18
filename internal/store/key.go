package store

import (
	"context"
	"crypto"
	"errors"

	"github.com/kubev2v/migration-planner/internal/store/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PrivateKey interface {
	Create(ctx context.Context, privateKey model.Key) (*model.Key, error)
	Get(ctx context.Context, orgID string) (*model.Key, error)
	Delete(ctx context.Context, orgID string) error
	GetPublicKey(ctx context.Context, id string) (crypto.PublicKey, error)
}

type PrivateKeyStore struct {
	db *gorm.DB
}

func NewPrivateKey(db *gorm.DB) PrivateKey {
	return &PrivateKeyStore{db: db}
}

func (p *PrivateKeyStore) Create(ctx context.Context, privateKey model.Key) (*model.Key, error) {
	result := p.getDB(ctx).Clauses(clause.Returning{}).Create(&privateKey)
	if result.Error != nil {
		return nil, result.Error
	}
	return &privateKey, nil
}

func (p *PrivateKeyStore) Get(ctx context.Context, orgID string) (*model.Key, error) {
	privateKey := model.Key{OrgID: orgID}
	result := p.getDB(ctx).First(&privateKey)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrRecordNotFound
		}
		return nil, result.Error
	}
	return &privateKey, nil
}

func (p *PrivateKeyStore) Delete(ctx context.Context, orgID string) error {
	privateKey := model.Key{OrgID: orgID}
	result := p.getDB(ctx).Unscoped().Delete(&privateKey)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		zap.S().Named("private_key_store").Infof("ERROR: %v", result.Error)
		return result.Error
	}
	return nil
}

func (p *PrivateKeyStore) GetPublicKey(ctx context.Context, kid string) (crypto.PublicKey, error) {
	key := model.Key{}
	if err := p.getDB(ctx).Where("id = ?", kid).First(&key).Error; err != nil {
		return nil, err
	}

	return key.PrivateKey.PublicKey, nil
}

func (p *PrivateKeyStore) getDB(ctx context.Context) *gorm.DB {
	tx := FromContext(ctx)
	if tx != nil {
		return tx
	}
	return p.db
}
