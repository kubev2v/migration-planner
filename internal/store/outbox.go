package store

import (
	"context"

	"github.com/kubev2v/migration-planner/internal/store/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Outbox interface {
	Insert(ctx context.Context, event model.OutboxEvent) error
	List(ctx context.Context) ([]model.OutboxEvent, error)
	Delete(ctx context.Context, ids ...int) error
}

type OutboxStore struct {
	db *gorm.DB
}

var _ Outbox = (*OutboxStore)(nil)

func NewOutboxStore(db *gorm.DB) Outbox {
	return &OutboxStore{db: db}
}

func (s *OutboxStore) Insert(ctx context.Context, event model.OutboxEvent) error {
	return s.getDB(ctx).Create(&event).Error
}

func (s *OutboxStore) List(ctx context.Context) ([]model.OutboxEvent, error) {
	var events []model.OutboxEvent
	result := s.getDB(ctx).
		Order("id ASC").
		Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Find(&events)
	if result.Error != nil {
		return nil, result.Error
	}
	return events, nil
}

func (s *OutboxStore) Delete(ctx context.Context, ids ...int) error {
	if len(ids) == 0 {
		return nil
	}
	return s.getDB(ctx).Where("id IN ?", ids).Delete(&model.OutboxEvent{}).Error
}

func (s *OutboxStore) getDB(ctx context.Context) *gorm.DB {
	tx := FromContext(ctx)
	if tx != nil {
		return tx
	}
	return s.db
}
