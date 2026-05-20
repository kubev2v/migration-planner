package events

import (
	"context"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/plugin/kprom"
	"go.uber.org/zap"
)

type Writer interface {
	Write(ctx context.Context, topic string, data []byte) error
}

type KafkaProducer struct {
	cl *kgo.Client
}

func NewKafkaProducer(brokers []string, opts ...kgo.Opt) (*KafkaProducer, error) {
	clientID, err := os.Hostname()
	if err != nil {
		clientID = fmt.Sprintf("producer-%s", uuid.NewString())
	}

	defaults := []kgo.Opt{
		kgo.SeedBrokers(brokers...),
		kgo.ClientID(clientID),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.WithHooks(kprom.NewMetrics("kafka_producer")),
	}

	cl, err := kgo.NewClient(append(defaults, opts...)...)
	if err != nil {
		return nil, fmt.Errorf("failed to create kafka producer: %w", err)
	}

	return &KafkaProducer{cl: cl}, nil
}

func (p *KafkaProducer) Write(ctx context.Context, topic string, data []byte) error {
	record := &kgo.Record{
		Topic: topic,
		Key:   []byte(uuid.New().String()),
		Value: data,
	}

	results := p.cl.ProduceSync(ctx, record)
	if err := results.FirstErr(); err != nil {
		return fmt.Errorf("failed to produce message: %w", err)
	}

	zap.S().Infow("message pushed to kafka", "topic", record.Topic, "offset", record.Offset, "partition", record.Partition)
	return nil
}

func (p *KafkaProducer) Ping(ctx context.Context) error {
	return p.cl.Ping(ctx)
}

func (p *KafkaProducer) Close() {
	p.cl.Close()
}

type NoOpWriter struct{}

func NewNoOpWriter() *NoOpWriter { return &NoOpWriter{} }

func (w *NoOpWriter) Write(ctx context.Context, topic string, data []byte) error { return nil }
