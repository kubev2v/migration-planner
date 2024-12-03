package events

import (
	"bytes"
	"context"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("producer", Ordered, func() {
	Context("write", func() {
		It("writes succsessfully", func() {
			w := newTestWriter()
			kp := NewEventProducer(w)

			// add the first message
			msg := []byte("msg1")
			err := kp.Write(context.TODO(), "topic1", bytes.NewReader(msg))
			Expect(err).To(BeNil())
			Expect(len(w.Messages)).To(Equal(1))
			Expect(w.Messages[0].Context.GetType()).To(Equal("topic1"))

			msg = []byte("msg2")
			err = kp.Write(context.TODO(), "topic2", bytes.NewReader(msg))
			Expect(err).To(BeNil())

			<-time.After(1 * time.Second)
			Expect(len(w.Messages)).To(Equal(2))

			kp.Close()
		})
	})
})

type testwriter struct {
	Messages []cloudevents.Event
}

func newTestWriter() *testwriter {
	return &testwriter{Messages: []cloudevents.Event{}}
}

func (t *testwriter) Write(ctx context.Context, topic string, e cloudevents.Event) error {
	t.Messages = append(t.Messages, e)
	return nil
}

func (t *testwriter) Close(_ context.Context) error {
	return nil
}
