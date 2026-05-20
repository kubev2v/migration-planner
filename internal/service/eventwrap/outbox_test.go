package eventwrap_test

import (
	"context"
	"fmt"

	"github.com/kubev2v/migration-planner/internal/service/eventwrap"
	"github.com/kubev2v/migration-planner/pkg/events"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("OutboxService", func() {
	var (
		outbox  *mockOutbox
		s       *mockStore
		service *eventwrap.OutboxService
	)

	BeforeEach(func() {
		outbox = &mockOutbox{}
		s = &mockStore{outbox: outbox}
		service = eventwrap.NewOutboxService(s)
	})

	It("inserts a valid event into the outbox", func() {
		err := service.Insert(context.Background(), events.VisitorEventType, []byte(`{"test":"data"}`))
		Expect(err).ToNot(HaveOccurred())
		Expect(outbox.events).To(HaveLen(1))
		Expect(outbox.events[0].EventType).To(Equal(events.VisitorEventType))
	})

	It("inserts an assessment created event", func() {
		err := service.Insert(context.Background(), events.AssessmentCreatedEventType, []byte(`{"test":"assessment"}`))
		Expect(err).ToNot(HaveOccurred())
		Expect(outbox.events).To(HaveLen(1))
		Expect(outbox.events[0].EventType).To(Equal(events.AssessmentCreatedEventType))
	})

	It("returns error when store insert fails", func() {
		outbox.insertErr = fmt.Errorf("db connection lost")
		err := service.Insert(context.Background(), events.VisitorEventType, []byte(`{"test":"data"}`))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("db connection lost"))
	})
})
