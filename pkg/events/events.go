package events

import "context"

const (
	EventTypeKafka        = "kafka-event"
	EventTypeNotification = "notification-event"
)

// Writer is a unified interface for writing events to different destinations
// (Kafka, notification service, HTTP endpoints, etc.)
type Writer interface {
	Write(ctx context.Context, data []byte) error
}
