package model

import "time"

type OutboxEvent struct {
	ID          int        `gorm:"primaryKey;autoIncrement"`
	EventType   string     `gorm:"column:event_type;not null;type:varchar(255)"`
	Payload     []byte     `gorm:"column:payload;not null;type:jsonb"`
	RetryCount  int        `gorm:"column:retry_count;not null;default:0"`
	NextRetryAt *time.Time `gorm:"column:next_retry_at"`
}

func (OutboxEvent) TableName() string {
	return "outbox_events"
}
