package notification_test

import (
	"github.com/kubev2v/migration-planner/pkg/events/notification"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("New", func() {
	It("stamps the fields fixed by the notification service registration", func() {
		n := notification.New(
			notification.PartnershipRequestEventType,
			"org-123",
			notification.SeverityImportant,
			map[string]string{"request_id": "req-1"},
			map[string]bool{"test": true},
			notification.Recipient{OnlyAdmins: true, IgnoreUserPreferences: true},
		)

		Expect(n.ID).NotTo(BeEmpty())
		Expect(n.Version).To(Equal("v2.0.0"))
		Expect(n.Bundle).To(Equal("openshift"))
		Expect(n.Application).To(Equal("migration-advisor"))
		Expect(n.EventType).To(Equal(notification.PartnershipRequestEventType))
		Expect(n.OrgID).To(Equal("org-123"))
		Expect(n.Severity).To(Equal(notification.SeverityImportant))
		Expect(n.Context).To(HaveKeyWithValue("request_id", "req-1"))
		Expect(n.Timestamp).NotTo(BeEmpty())
	})

	It("wraps the payload as a single event with empty metadata", func() {
		n := notification.New(notification.AssessmentSharedEventType, "org-1", notification.SeverityImportant, nil, map[string]bool{"test": true})

		Expect(n.Events).To(HaveLen(1))
		Expect(n.Events[0].Metadata).To(BeEmpty())
		Expect(n.Events[0].Payload).To(Equal(map[string]bool{"test": true}))
	})

	It("carries through the given recipients", func() {
		n := notification.New(notification.PartnershipResponseEventType, "org-1", notification.SeverityImportant, nil, nil,
			notification.Recipient{Users: []string{"alice"}})

		Expect(n.Recipients).To(HaveLen(1))
		Expect(n.Recipients[0].Users).To(ConsistOf("alice"))
	})

	It("generates a unique id per call", func() {
		a := notification.New(notification.AssessmentCreatedEventType, "org", notification.SeverityImportant, nil, nil)
		b := notification.New(notification.AssessmentCreatedEventType, "org", notification.SeverityImportant, nil, nil)

		Expect(a.ID).NotTo(Equal(b.ID))
	})
})
