package eventwrap

import (
	"context"
	"time"

	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/pkg/events/kafka"
	"github.com/kubev2v/migration-planner/pkg/events/notification"
	"go.uber.org/zap"
)

type OutboxDispatcher struct {
	store    store.Store
	writer   kafka.Writer
	notifier notification.Writer
	interval time.Duration
}

type writerType int

const (
	writerTypeUnknown writerType = iota
	writerTypeKafka
	writerTypeNotification

	outboxMaxRetries        = 10
	outboxBackoffBase       = 5
	outboxBackoffCapSeconds = 3 * 60 * 60 // 3h
)

func NewOutboxDispatcher(s store.Store, writer kafka.Writer, notifier notification.Writer, interval time.Duration) *OutboxDispatcher {
	return &OutboxDispatcher{
		store:    s,
		writer:   writer,
		notifier: notifier,
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

	var toDelete, toRetain []int
	for _, outboxEvent := range outboxEvents {
		if outboxEvent.RetryCount >= outboxMaxRetries {
			zap.S().Warnw(
				"outbox dispatcher: max retries exceeded",
				"id", outboxEvent.ID,
				"event_type", outboxEvent.EventType,
				"retries", outboxEvent.RetryCount,
			)
			toDelete = append(toDelete, outboxEvent.ID)
			continue
		}

		var err error
		switch writerTypeForEventType(outboxEvent.EventType) {
		case writerTypeKafka:
			err = d.writer.Write(ctx, kafka.GenericTopic, outboxEvent.Payload)
		case writerTypeNotification:
			err = d.notifier.Write(ctx, outboxEvent.Payload)
		default:
			zap.S().Errorw("outbox dispatcher: no writer registered for event type, dropping",
				"id", outboxEvent.ID, "event_type", outboxEvent.EventType)
			toDelete = append(toDelete, outboxEvent.ID)
			continue
		}
		if err != nil {
			zap.S().Errorw(
				"outbox dispatcher: failed to write event, will retry",
				"id", outboxEvent.ID,
				"event_type", outboxEvent.EventType,
				"error", err,
			)
			toRetain = append(toRetain, outboxEvent.ID)
			continue
		}

		zap.S().Debugw(
			"successfully dispatched",
			"id", outboxEvent.ID,
			"event_type", outboxEvent.EventType,
		)
		toDelete = append(toDelete, outboxEvent.ID)
	}

	if err := d.store.Outbox().Delete(ctx, toDelete...); err != nil {
		zap.S().Errorw("outbox dispatcher: failed to delete toDelete events", "error", err)
	}

	if err := d.store.Outbox().MarkFailed(ctx, outboxBackoffBase, outboxBackoffCapSeconds, toRetain...); err != nil {
		zap.S().Errorw("outbox dispatcher: failed to record retry backoff", "error", err)
	}

	if _, err := store.Commit(ctx); err != nil {
		zap.S().Errorw("outbox dispatcher: failed to commit transaction", "error", err)
	}
}

// writerTypeForEventType classifies an outbox event type so the dispatcher
// knows which writer to hand it to.
func writerTypeForEventType(eventType string) writerType {
	switch eventType {
	case kafka.AssessmentCreatedEventType, kafka.AssessmentDeletedEventType, kafka.PartnerCustomerEventType,
		kafka.ShareAssessmentEventType, kafka.UnshareAssessmentEventType, kafka.SizingEventType,
		kafka.MigrationComplexityEventType, kafka.MigrationTimeEstimationEventType,
		kafka.DownloadOVAEventType, kafka.VisitorEventType:
		return writerTypeKafka
	case notification.PartnershipRequestEventType, notification.PartnershipResponseEventType,
		notification.AssessmentSharedEventType, notification.AssessmentCreatedEventType:
		return writerTypeNotification
	default:
		return writerTypeUnknown
	}
}
