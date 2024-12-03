package events

import (
	"context"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"go.uber.org/zap"
)

// event writer used in dev
type StdoutWriter struct{}

func (s *StdoutWriter) Write(ctx context.Context, topic string, e cloudevents.Event) error {
	zap.S().Named("stout_writer").Infow("event wrote", "event", e, "topic", topic)
	return nil
}

func (s *StdoutWriter) Close(_ context.Context) error {
	return nil
}
