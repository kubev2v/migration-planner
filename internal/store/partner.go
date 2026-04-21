package store

import (
	"context"
	"errors"

	"github.com/kubev2v/migration-planner/internal/store/model"
	"gorm.io/gorm"
)

type PartnerCustomer interface {
	List(ctx context.Context, filter *PartnerQueryFilter) (model.PartnerCustomerList, error)
	Create(ctx context.Context, pc model.PartnerCustomer) (*model.PartnerCustomer, error)
	Get(ctx context.Context, filter *PartnerQueryFilter) (*model.PartnerCustomer, error)
	Update(ctx context.Context, pc model.PartnerCustomer) (*model.PartnerCustomer, error)
}

type PartnerCustomerStore struct {
	db *gorm.DB
}

var _ PartnerCustomer = (*PartnerCustomerStore)(nil)

func NewPartnerCustomerStore(db *gorm.DB) PartnerCustomer {
	return &PartnerCustomerStore{db: db}
}

func (p *PartnerCustomerStore) List(ctx context.Context, filter *PartnerQueryFilter) (model.PartnerCustomerList, error) {
	var partners model.PartnerCustomerList
	tx := p.getDB(ctx).Model(&partners).Preload("Partner")

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

func (p *PartnerCustomerStore) Create(ctx context.Context, pc model.PartnerCustomer) (*model.PartnerCustomer, error) {
	var created model.PartnerCustomer
	err := p.getDB(ctx).Transaction(func(tx *gorm.DB) error {
		if result := tx.Create(&pc); result.Error != nil {
			if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
				return ErrDuplicateKey
			}
			return result.Error
		}
		result := tx.Preload("Partner").First(&created, "id = ?", pc.ID)
		return result.Error
	})
	if err != nil {
		return nil, err
	}
	return &created, nil
}

func (p *PartnerCustomerStore) Get(ctx context.Context, filter *PartnerQueryFilter) (*model.PartnerCustomer, error) {
	var pc model.PartnerCustomer
	tx := p.getDB(ctx).Preload("Partner")

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

func (p *PartnerCustomerStore) Update(ctx context.Context, pc model.PartnerCustomer) (*model.PartnerCustomer, error) {
	var updated model.PartnerCustomer
	err := p.getDB(ctx).Transaction(func(tx *gorm.DB) error {
		if result := tx.Model(&pc).Select("request_status", "reason", "accepted_at", "terminated_at").Updates(&pc); result.Error != nil {
			return result.Error
		}
		result := tx.Preload("Partner").First(&updated, "id = ?", pc.ID)
		return result.Error
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

func (p *PartnerCustomerStore) getDB(ctx context.Context) *gorm.DB {
	tx := FromContext(ctx)
	if tx != nil {
		return tx
	}
	return p.db
}
