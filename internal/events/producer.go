package events

import (
	"context"
	"io"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

const (
	InventoryMessageKind string = "assisted.migrations.events.inventory"
	AgentMessageKind     string = "assisted.migrations.events.agent"
	UIMessageKind        string = "assisted.migrations.events.ui"
	defaultTopic         string = "assisted.migrations.events"
)

// Writer is the interface to be implemented by the underlying writer.
type Writer interface {
	Write(ctx context.Context, topic string, e cloudevents.Event) error
	Close(ctx context.Context) error
}

// EventProducer is a wrapper around a Writer with the buffer.
// It has a buffer to store pending events to not block the caller if the writer takes time to write the event.
type EventProducer struct {
	buffer           *buffer
	startConsumingCh chan any
	doneCh           chan any
	writer           Writer
	topic            string
}

func NewEventProducer(w Writer, opts ...ProducerOptions) *EventProducer {
	ep := &EventProducer{
		buffer:           newBuffer(),
		startConsumingCh: make(chan any),
		doneCh:           make(chan any),
		writer:           w,
		topic:            defaultTopic,
	}

	for _, o := range opts {
		o(ep)
	}

	go ep.run()
	return ep
}

func (ep *EventProducer) Write(ctx context.Context, kind string, body io.Reader) error {
	d, err := io.ReadAll(body)
	if err != nil {
		return err
	}

	prevSize := ep.buffer.Size()
	if err := ep.buffer.PushBack(&message{
		Kind: kind,
		Data: d,
	}); err != nil {
		return err
	}

	if prevSize == 0 {
		// unblock the producer and start sending messages
		ep.startConsumingCh <- struct{}{}
	}

	return nil
}

func (ep *EventProducer) Close() error {
	closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	g, ctx := errgroup.WithContext(closeCtx)
	g.Go(func() error {
		ep.doneCh <- struct{}{}
		return ep.writer.Close(ctx)
	})
	if err := g.Wait(); err != nil {
		zap.S().Errorf("event producer closed with error: %s", err)
		return err
	}

	zap.S().Named("event producer").Info("event producer closed")

	return nil
}

func (ep *EventProducer) run() {
	for {
		select {
		case <-ep.doneCh:
			return
		default:
		}

		if ep.buffer.Size() == 0 {
			select {
			case <-ep.startConsumingCh:
			case <-ep.doneCh:
			}
		}

		msg := ep.buffer.Pop()
		if msg == nil {
			continue
		}

		e := cloudevents.NewEvent()
		e.SetID(uuid.NewString())
		e.SetSource("assisted.migrations.planner")
		e.SetType(string(msg.Kind))
		_ = e.SetData(*cloudevents.StringOfApplicationJSON(), msg.Data)

		if err := ep.writer.Write(context.TODO(), ep.topic, e); err != nil {
			zap.S().Named("kafka_producer").Errorw("failed to send message", "error", err, "event", e)
		}
	}
}
