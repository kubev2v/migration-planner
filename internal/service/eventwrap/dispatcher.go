package eventwrap

import (
	"context"
	"time"

	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/pkg/events"
	"go.uber.org/zap"
)

type OutboxDispatcher struct {
	store    store.Store
	writer   events.Writer
	interval time.Duration
}

func NewOutboxDispatcher(s store.Store, writer events.Writer, interval time.Duration) *OutboxDispatcher {
	return &OutboxDispatcher{
		store:    s,
		writer:   writer,
		interval: interval,
	}
}

func (d *OutboxDispatcher) Run(ctx context.Context) {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.processBatch(ctx)
		}
	}
}

func (d *OutboxDispatcher) processBatch(ctx context.Context) {
	ctx, err := d.store.NewTransactionContext(ctx)
	if err != nil {
		zap.S().Warnw("outbox dispatcher: failed to start transaction", "error", err)
		return
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	outboxEvents, err := d.store.Outbox().List(ctx)
	if err != nil {
		zap.S().Errorw("outbox dispatcher: failed to list events", "error", err)
		return
	}

	if len(outboxEvents) == 0 {
		return
	}

	var published []int
	for _, outboxEvent := range outboxEvents {
		if err := d.writer.Write(ctx, events.GenericTopic, outboxEvent.Payload); err != nil {
			zap.S().Errorw("outbox dispatcher: failed to write event, will retry",
				"id", outboxEvent.ID, "event_type", outboxEvent.EventType, "error", err)
			continue
		}
		published = append(published, outboxEvent.ID)
	}

	if err := d.store.Outbox().Delete(ctx, published...); err != nil {
		zap.S().Errorw("outbox dispatcher: failed to delete published events", "error", err)
	}

	if _, err := store.Commit(ctx); err != nil {
		zap.S().Errorw("outbox dispatcher: failed to commit transaction", "error", err)
	}
}
