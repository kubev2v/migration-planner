package eventwrap

import (
	"context"
	"strings"
	"time"

	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/pkg/metrics"
	"go.uber.org/zap"
)

type OutboxDispatcher struct {
	store    store.Store
	writers  map[string][]Writer
	interval time.Duration
}

type outcome struct {
	integration string
	result      string
}

const (
	outboxMaxRetries        = 10
	outboxBackoffBase       = 5
	outboxBackoffCapSeconds = 3 * 60 * 60 // 3h
)

func NewOutboxDispatcher(s store.Store, writerRegistry map[string][]Writer, interval time.Duration) *OutboxDispatcher {
	return &OutboxDispatcher{
		store:    s,
		writers:  writerRegistry,
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
	var completedResults []outcome
	for _, outboxEvent := range outboxEvents {
		writers, ok := d.writers[outboxEvent.EventType]
		if !ok || len(writers) == 0 {
			zap.S().Errorw("outbox dispatcher: no writer registered for event type, dropping",
				"id", outboxEvent.ID, "event_type", outboxEvent.EventType)
			toDelete = append(toDelete, outboxEvent.ID)
			continue
		}

		// Write to all registered writers, collect errors
		var results []outcome
		var errs []error
		for _, writer := range writers {
			rl := metrics.IntegrationResultSuccess
			if err := writer.Write(ctx, outboxEvent.Payload); err != nil {
				errs = append(errs, err)
				rl = metrics.IntegrationResultFailure
			}
			results = append(results, outcome{strings.Split(outboxEvent.EventType, "-")[0], rl})
		}

		outboxEvent.RetryCount++

		if len(errs) == 0 {
			zap.S().Debugw(
				"successfully dispatched",
				"id", outboxEvent.ID,
				"event_type", outboxEvent.EventType,
			)
			toDelete = append(toDelete, outboxEvent.ID)
			completedResults = append(completedResults, results...)
			continue
		}

		// has errors but not yet exceeds max retries
		if !outboxEvent.Exceeds(outboxMaxRetries) {
			zap.S().Errorw(
				"outbox dispatcher: failed to write event, will retry",
				"id", outboxEvent.ID,
				"event_type", outboxEvent.EventType,
				"errors", errs,
				"failed_count", len(errs),
				"total_writers", len(writers),
			)
			toRetain = append(toRetain, outboxEvent.ID)
			continue
		}

		zap.S().Warnw(
			"outbox dispatcher: max retries exceeded",
			"id", outboxEvent.ID,
			"event_type", outboxEvent.EventType,
			"retries", outboxEvent.RetryCount,
		)

		toDelete = append(toDelete, outboxEvent.ID)
		completedResults = append(completedResults, results...)
	}

	if err := d.store.Outbox().Delete(ctx, toDelete...); err != nil {
		zap.S().Errorw("outbox dispatcher: failed to delete toDelete events", "error", err)
		return
	}

	if err := d.store.Outbox().MarkFailed(ctx, outboxBackoffBase, outboxBackoffCapSeconds, toRetain...); err != nil {
		zap.S().Errorw("outbox dispatcher: failed to record retry backoff", "error", err)
		return
	}

	if _, err := store.Commit(ctx); err != nil {
		zap.S().Errorw("outbox dispatcher: failed to commit transaction", "error", err)
		return
	}

	for _, o := range completedResults {
		metrics.RecordIntegrationResult(o.integration, o.result)
	}
}
