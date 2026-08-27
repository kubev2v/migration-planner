package eventwrap_test

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/kubev2v/migration-planner/internal/service/eventwrap"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/events"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type mockOutbox struct {
	events      []model.OutboxEvent
	deletedIDs  []int
	failedIDs   []int
	listErr     error
	deleteErr   error
	insertErr   error
	markFailErr error
}

func (m *mockOutbox) Insert(_ context.Context, event model.OutboxEvent) error {
	if m.insertErr != nil {
		return m.insertErr
	}
	event.ID = len(m.events) + 1
	m.events = append(m.events, event)
	return nil
}

func (m *mockOutbox) List(_ context.Context) ([]model.OutboxEvent, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.events, nil
}

func (m *mockOutbox) Delete(_ context.Context, ids ...int) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.deletedIDs = append(m.deletedIDs, ids...)
	remaining := make([]model.OutboxEvent, 0)
	deleteSet := make(map[int]bool)
	for _, id := range ids {
		deleteSet[id] = true
	}
	for _, e := range m.events {
		if !deleteSet[e.ID] {
			remaining = append(remaining, e)
		}
	}
	m.events = remaining
	return nil
}

func (m *mockOutbox) MarkFailed(_ context.Context, _, _ int, ids ...int) error {
	if m.markFailErr != nil {
		return m.markFailErr
	}
	m.failedIDs = append(m.failedIDs, ids...)
	for i := range m.events {
		if slices.Contains(ids, m.events[i].ID) {
			m.events[i].RetryCount++
		}
	}
	return nil
}

type mockStore struct {
	outbox *mockOutbox
}

func (m *mockStore) NewTransactionContext(ctx context.Context) (context.Context, error) {
	return ctx, nil
}

func (m *mockStore) Outbox() store.Outbox                                       { return m.outbox }
func (m *mockStore) Agent() store.Agent                                         { return nil }
func (m *mockStore) Authz() store.Authz                                         { return nil }
func (m *mockStore) Source() store.Source                                       { return nil }
func (m *mockStore) SourceSubsetInventory() store.SourceSubsetInventory         { return nil }
func (m *mockStore) AssessmentSubsetInventory() store.AssessmentSubsetInventory { return nil }
func (m *mockStore) ImageInfra() store.ImageInfra                               { return nil }
func (m *mockStore) PrivateKey() store.PrivateKey                               { return nil }
func (m *mockStore) Label() store.Label                                         { return nil }
func (m *mockStore) Assessment() store.Assessment                               { return nil }
func (m *mockStore) ClusterSizingInput() store.ClusterSizingInput               { return nil }
func (m *mockStore) AssessmentEnhancementData() store.AssessmentEnhancementData { return nil }
func (m *mockStore) Job() store.Job                                             { return nil }
func (m *mockStore) Accounts() store.Accounts                                   { return nil }
func (m *mockStore) PartnerCustomer() store.PartnerCustomer                     { return nil }
func (m *mockStore) Statistics(_ context.Context) (model.InventoryStats, error) {
	return model.InventoryStats{}, nil
}
func (m *mockStore) RequestMetricsCacheRefresh() {}
func (m *mockStore) Close() error                { return nil }

type mockWriter struct {
	written  [][]byte
	writeErr error
}

func (m *mockWriter) Write(_ context.Context, data []byte) error {
	if m.writeErr != nil {
		return m.writeErr
	}
	m.written = append(m.written, data)
	return nil
}

var _ = Describe("OutboxDispatcher", func() {
	var (
		outbox      *mockOutbox
		s           *mockStore
		kafkaWriter *mockWriter
		notifWriter *mockWriter
	)

	BeforeEach(func() {
		outbox = &mockOutbox{}
		s = &mockStore{outbox: outbox}
		kafkaWriter = &mockWriter{}
		notifWriter = &mockWriter{}
	})

	runOneTick := func() {
		writerRegistry := map[string][]events.Writer{
			events.EventTypeKafka:        {kafkaWriter},
			events.EventTypeNotification: {notifWriter},
		}
		dispatcher := eventwrap.NewOutboxDispatcher(s, writerRegistry, 10*time.Millisecond)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		dispatcher.Run(ctx)
	}

	It("writes events and deletes them from outbox", func() {
		outbox.events = []model.OutboxEvent{
			{ID: 1, EventType: events.EventTypeKafka, Payload: []byte("event-data-1")},
		}

		runOneTick()

		Expect(kafkaWriter.written).To(HaveLen(1))
		Expect(kafkaWriter.written[0]).To(Equal([]byte("event-data-1")))
		Expect(outbox.events).To(BeEmpty())
	})

	It("does nothing when outbox is empty", func() {
		runOneTick()

		Expect(kafkaWriter.written).To(BeNil())
		Expect(outbox.deletedIDs).To(BeNil())
	})

	It("backs off events that fail to write and retains them", func() {
		id := 17
		outbox.events = []model.OutboxEvent{
			{ID: id, EventType: events.EventTypeKafka, Payload: []byte("event-data-1")},
		}
		kafkaWriter.writeErr = fmt.Errorf("kafka unavailable")

		runOneTick()

		Expect(kafkaWriter.written).To(BeNil())
		Expect(outbox.events).To(HaveLen(1))
		Expect(outbox.deletedIDs).To(BeEmpty())
		Expect(outbox.failedIDs).To(ContainElement(id))
	})

	It("drops events that exceeded the max retry count without writing them", func() {
		outbox.events = []model.OutboxEvent{
			{ID: 1, EventType: events.EventTypeKafka, Payload: []byte("poison"), RetryCount: 15},
			{ID: 2, EventType: events.EventTypeKafka, Payload: []byte("fresh")},
		}

		runOneTick()

		Expect(kafkaWriter.written).To(HaveLen(1))
		Expect(kafkaWriter.written[0]).To(Equal([]byte("fresh")))
		Expect(outbox.deletedIDs).To(ContainElement(1))
		Expect(outbox.failedIDs).To(BeEmpty())
		Expect(outbox.events).To(BeEmpty())
	})

	It("writes multiple events and bulk-deletes", func() {
		outbox.events = []model.OutboxEvent{
			{ID: 1, EventType: events.EventTypeKafka, Payload: []byte("event-data-1")},
			{ID: 2, EventType: events.EventTypeKafka, Payload: []byte("event-data-2")},
		}

		runOneTick()

		Expect(kafkaWriter.written).To(HaveLen(2))
		Expect(outbox.events).To(BeEmpty())
	})

	It("routes notification-service events to the notifier, not the Kafka writer", func() {
		outbox.events = []model.OutboxEvent{
			{ID: 1, EventType: events.EventTypeNotification, Payload: []byte("notification-data")},
			{ID: 2, EventType: events.EventTypeKafka, Payload: []byte("event-data")},
		}

		runOneTick()

		Expect(notifWriter.written).To(HaveLen(1))
		Expect(notifWriter.written[0]).To(Equal([]byte("notification-data")))
		Expect(kafkaWriter.written).To(HaveLen(1))
		Expect(kafkaWriter.written[0]).To(Equal([]byte("event-data")))
		Expect(outbox.events).To(BeEmpty())
	})

	It("drop events with an unrecognized event type", func() {
		outbox.events = []model.OutboxEvent{
			{ID: 1, EventType: "some.unknown.type", Payload: []byte("event-data")},
		}

		runOneTick()

		Expect(kafkaWriter.written).To(BeNil())
		Expect(notifWriter.written).To(BeNil())
		Expect(outbox.events).To(HaveLen(0))
		Expect(outbox.deletedIDs).To(ConsistOf(1))
		Expect(outbox.failedIDs).To(BeEmpty())
	})
})
