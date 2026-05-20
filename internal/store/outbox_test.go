package store_test

import (
	"context"
	"sync"

	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

var _ = Describe("outbox store", Ordered, func() {
	var (
		s      store.Store
		gormdb *gorm.DB
	)

	BeforeAll(func() {
		cfg, err := config.New()
		Expect(err).To(BeNil())
		db, err := store.InitDB(cfg)
		Expect(err).To(BeNil())

		s = store.NewStore(db)
		gormdb = db
	})

	AfterAll(func() {
		_ = s.Close()
	})

	AfterEach(func() {
		gormdb.Exec("DELETE FROM outbox_events;")
	})

	Context("Insert", func() {
		It("successfully inserts an event", func() {
			err := s.Outbox().Insert(context.TODO(), model.OutboxEvent{
				EventType: "test.event",
				Payload:   []byte(`{"key":"value"}`),
			})
			Expect(err).To(BeNil())

			events, err := s.Outbox().List(context.TODO())
			Expect(err).To(BeNil())
			Expect(events).To(HaveLen(1))
			Expect(events[0].EventType).To(Equal("test.event"))
			Expect(events[0].Payload).To(MatchJSON(`{"key":"value"}`))
			Expect(events[0].ID).To(BeNumerically(">", 0))
		})

		It("auto-increments IDs", func() {
			err := s.Outbox().Insert(context.TODO(), model.OutboxEvent{
				EventType: "event.1",
				Payload:   []byte(`{}`),
			})
			Expect(err).To(BeNil())

			err = s.Outbox().Insert(context.TODO(), model.OutboxEvent{
				EventType: "event.2",
				Payload:   []byte(`{}`),
			})
			Expect(err).To(BeNil())

			events, err := s.Outbox().List(context.TODO())
			Expect(err).To(BeNil())
			Expect(events).To(HaveLen(2))
			Expect(events[0].ID).To(BeNumerically("<", events[1].ID))
		})
	})

	Context("List", func() {
		It("returns events ordered by ID ascending", func() {
			for i := range 3 {
				err := s.Outbox().Insert(context.TODO(), model.OutboxEvent{
					EventType: "test.event",
					Payload:   []byte(`{}`),
				})
				Expect(err).To(BeNil())
				_ = i
			}

			events, err := s.Outbox().List(context.TODO())
			Expect(err).To(BeNil())
			Expect(events).To(HaveLen(3))
			Expect(events[0].ID).To(BeNumerically("<", events[1].ID))
			Expect(events[1].ID).To(BeNumerically("<", events[2].ID))
		})

		It("returns empty slice when no events exist", func() {
			events, err := s.Outbox().List(context.TODO())
			Expect(err).To(BeNil())
			Expect(events).To(BeEmpty())
		})
	})

	Context("Delete", func() {
		It("deletes a single event by ID", func() {
			err := s.Outbox().Insert(context.TODO(), model.OutboxEvent{
				EventType: "test.event",
				Payload:   []byte(`{}`),
			})
			Expect(err).To(BeNil())

			events, err := s.Outbox().List(context.TODO())
			Expect(err).To(BeNil())
			Expect(events).To(HaveLen(1))

			err = s.Outbox().Delete(context.TODO(), events[0].ID)
			Expect(err).To(BeNil())

			events, err = s.Outbox().List(context.TODO())
			Expect(err).To(BeNil())
			Expect(events).To(BeEmpty())
		})

		It("deletes multiple events by IDs", func() {
			for range 3 {
				err := s.Outbox().Insert(context.TODO(), model.OutboxEvent{
					EventType: "test.event",
					Payload:   []byte(`{}`),
				})
				Expect(err).To(BeNil())
			}

			events, err := s.Outbox().List(context.TODO())
			Expect(err).To(BeNil())
			Expect(events).To(HaveLen(3))

			err = s.Outbox().Delete(context.TODO(), events[0].ID, events[2].ID)
			Expect(err).To(BeNil())

			remaining, err := s.Outbox().List(context.TODO())
			Expect(err).To(BeNil())
			Expect(remaining).To(HaveLen(1))
			Expect(remaining[0].ID).To(Equal(events[1].ID))
		})

		It("is a no-op when called with no IDs", func() {
			err := s.Outbox().Insert(context.TODO(), model.OutboxEvent{
				EventType: "test.event",
				Payload:   []byte(`{}`),
			})
			Expect(err).To(BeNil())

			err = s.Outbox().Delete(context.TODO())
			Expect(err).To(BeNil())

			events, err := s.Outbox().List(context.TODO())
			Expect(err).To(BeNil())
			Expect(events).To(HaveLen(1))
		})
	})

	Context("parallel reads with FOR UPDATE SKIP LOCKED", func() {
		const numReaders = 5
		const numEvents = 20

		It("concurrent transactions do not see the same events", func() {
			for range numEvents {
				err := s.Outbox().Insert(context.TODO(), model.OutboxEvent{
					EventType: "test.event",
					Payload:   []byte(`{}`),
				})
				Expect(err).To(BeNil())
			}

			var (
				mu      sync.Mutex
				results [numReaders][]model.OutboxEvent
				wg      sync.WaitGroup
				barrier sync.WaitGroup
			)

			barrier.Add(numReaders)

			for i := range numReaders {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					defer GinkgoRecover()

					ctx, err := s.NewTransactionContext(context.TODO())
					Expect(err).To(BeNil())

					events, err := s.Outbox().List(ctx)
					Expect(err).To(BeNil())

					mu.Lock()
					results[idx] = events
					mu.Unlock()

					barrier.Done()
					barrier.Wait()

					_, _ = store.Rollback(ctx)
				}(i)
			}

			wg.Wait()

			allIDs := make(map[int]bool)
			for _, r := range results {
				for _, e := range r {
					Expect(allIDs[e.ID]).To(BeFalse(), "event ID %d was read by multiple goroutines", e.ID)
					allIDs[e.ID] = true
				}
			}

			Expect(len(allIDs)).To(Equal(numEvents))
		})

		It("concurrent reads and writes do not produce duplicates", func() {
			for range numEvents {
				err := s.Outbox().Insert(context.TODO(), model.OutboxEvent{
					EventType: "test.event",
					Payload:   []byte(`{}`),
				})
				Expect(err).To(BeNil())
			}

			const extraEvents = 10

			var (
				mu      sync.Mutex
				results [numReaders][]model.OutboxEvent
				wg      sync.WaitGroup
				barrier sync.WaitGroup
				startCh = make(chan struct{})
			)

			barrier.Add(numReaders)

			// writer goroutine: inserts more events while readers are active
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer GinkgoRecover()

				<-startCh
				for range extraEvents {
					err := s.Outbox().Insert(context.TODO(), model.OutboxEvent{
						EventType: "test.event.concurrent",
						Payload:   []byte(`{}`),
					})
					Expect(err).To(BeNil())
				}
			}()

			for i := range numReaders {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					defer GinkgoRecover()

					ctx, err := s.NewTransactionContext(context.TODO())
					Expect(err).To(BeNil())

					// signal the writer to start once the first reader opens a transaction
					if idx == 0 {
						close(startCh)
					}

					events, err := s.Outbox().List(ctx)
					Expect(err).To(BeNil())

					mu.Lock()
					results[idx] = events
					mu.Unlock()

					barrier.Done()
					barrier.Wait()

					_, _ = store.Rollback(ctx)
				}(i)
			}

			wg.Wait()

			allIDs := make(map[int]bool)
			for _, r := range results {
				for _, e := range r {
					Expect(allIDs[e.ID]).To(BeFalse(), "event ID %d was read by multiple goroutines", e.ID)
					allIDs[e.ID] = true
				}
			}

			Expect(len(allIDs)).To(BeNumerically(">=", numEvents))
			Expect(len(allIDs)).To(BeNumerically("<=", numEvents+extraEvents))
		})
	})
})
