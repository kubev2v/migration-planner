package kafka

import (
	"context"

	"github.com/kubev2v/migration-event-streamer/pkg/kafka"
	"github.com/twmb/franz-go/pkg/kgo"
)

type NoOpWriter struct{}

func NewNoOpWriter() *NoOpWriter { return &NoOpWriter{} }

func (w *NoOpWriter) Write(_ context.Context, _ []byte) error { return nil }

// TopicBoundWriter wraps KafkaProducer and binds it to a specific topic,
// implementing the unified events.Writer interface.
type TopicBoundWriter struct {
	producer *kafka.KafkaProducer
	topic    string
}

// NewTopicBoundProducer creates a new KafkaProducer bound to a specific topic.
func NewTopicBoundProducer(brokers []string, topic string, opts ...kgo.Opt) (*TopicBoundWriter, error) {
	producer, err := kafka.NewKafkaProducer(brokers, opts...)
	if err != nil {
		return nil, err
	}
	return &TopicBoundWriter{
		producer: producer,
		topic:    topic,
	}, nil
}

// Write sends data to the configured topic, implementing events.Writer.
func (w *TopicBoundWriter) Write(ctx context.Context, data []byte) error {
	return w.producer.Write(ctx, w.topic, data)
}

// Ping checks connectivity to Kafka brokers.
func (w *TopicBoundWriter) Ping(ctx context.Context) error {
	return w.producer.Ping(ctx)
}

// Close closes the underlying Kafka producer.
func (w *TopicBoundWriter) Close() {
	w.producer.Close()
}
