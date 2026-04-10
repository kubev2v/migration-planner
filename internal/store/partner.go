package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Partner interface {
	List(ctx context.Context, filter *PartnerQueryFilter) (model.PartnerCustomerList, error)
	Create(ctx context.Context, pc model.PartnerCustomer) (*model.PartnerCustomer, error)
	Get(ctx context.Context, filter *PartnerQueryFilter) (*model.PartnerCustomer, error)
	Update(ctx context.Context, pc model.PartnerCustomer) (*model.PartnerCustomer, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type PartnerStore struct {
	db *gorm.DB
}

var _ Partner = (*PartnerStore)(nil)

func NewPartnerStore(db *gorm.DB) Partner {
	return &PartnerStore{db: db}
}

func (p *PartnerStore) List(ctx context.Context, filter *PartnerQueryFilter) (model.PartnerCustomerList, error) {
	var partners model.PartnerCustomerList
	tx := p.getDB(ctx).Model(&partners)

	if filter != nil {
		for _, fn := range filter.QueryFn {
			tx = fn(tx)
		}
	}

	result := tx.Find(&partners)
	if result.Error != nil {
		return nil, result.Error
	}
	return partners, nil
}

func (p *PartnerStore) Create(ctx context.Context, pc model.PartnerCustomer) (*model.PartnerCustomer, error) {
	result := p.getDB(ctx).Clauses(clause.Returning{}).Create(&pc)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return nil, ErrDuplicateKey
		}
		return nil, result.Error
	}
	return &pc, nil
}

func (p *PartnerStore) Get(ctx context.Context, filter *PartnerQueryFilter) (*model.PartnerCustomer, error) {
	var pc model.PartnerCustomer
	tx := p.getDB(ctx)

	if filter != nil {
		for _, fn := range filter.QueryFn {
			tx = fn(tx)
		}
	}

	result := tx.First(&pc)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrRecordNotFound
		}
		return nil, result.Error
	}
	return &pc, nil
}

func (p *PartnerStore) Update(ctx context.Context, pc model.PartnerCustomer) (*model.PartnerCustomer, error) {
	result := p.getDB(ctx).Model(&pc).Clauses(clause.Returning{}).Select("request_status", "reason").Updates(&pc)
	if result.Error != nil {
		return nil, result.Error
	}
	return &pc, nil
}

func (p *PartnerStore) Delete(ctx context.Context, id uuid.UUID) error {
	result := p.getDB(ctx).Unscoped().Delete(&model.PartnerCustomer{}, "id = ?", id)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return result.Error
	}
	return nil
}

func (p *PartnerStore) getDB(ctx context.Context) *gorm.DB {
	tx := FromContext(ctx)
	if tx != nil {
		return tx
	}
	return p.db
}
