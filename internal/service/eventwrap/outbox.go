package eventwrap

import (
	"context"
	"fmt"

	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

type OutboxService struct {
	store store.Store
}

func NewOutboxService(s store.Store) *OutboxService {
	return &OutboxService{store: s}
}

func (o *OutboxService) Insert(ctx context.Context, eventType string, payload []byte) error {
	if err := o.store.Outbox().Insert(ctx, model.OutboxEvent{
		EventType: eventType,
		Payload:   payload,
	}); err != nil {
		return fmt.Errorf("failed to write event %s to outbox: %w", eventType, err)
	}
	return nil
}
