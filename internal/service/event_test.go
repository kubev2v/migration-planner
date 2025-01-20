package service_test

import (
	"context"
	"encoding/json"
	"reflect"
	"time"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/events"
	"github.com/kubev2v/migration-planner/internal/service"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("event handler", Ordered, func() {
	Context("push events", func() {
		It("successfully push events", func() {
			eventWriter := newTestWriter()

			reqBody := api.Event{
				CreatedAt: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
				Data: []api.EventData{
					{Key: "key", Value: "value"},
				},
			}

			srv := service.NewServiceHandler(nil, events.NewEventProducer(eventWriter))
			resp, err := srv.PushEvents(context.TODO(), server.PushEventsRequestObject{
				Body: &reqBody,
			})

			Expect(err).To(BeNil())
			Expect(reflect.TypeOf(resp)).To(Equal(reflect.TypeOf(server.PushEvents201JSONResponse{})))

			<-time.After(500 * time.Millisecond)

			Expect(eventWriter.Messages).To(HaveLen(1))
			e := eventWriter.Messages[0]

			Expect(e.Type()).To(Equal(events.UIMessageKind))

			ev := &events.UIEvent{}
			err = json.Unmarshal(e.Data(), &ev)
			Expect(err).To(BeNil())
			Expect(ev.Data).To(HaveLen(1))
			Expect(ev.CreatedAt).To(Equal(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)))
			Expect(ev.Data["key"]).To(Equal("value"))
		})
	})
})
