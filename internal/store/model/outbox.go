package model

type OutboxEvent struct {
	ID        int    `gorm:"primaryKey;autoIncrement"`
	EventType string `gorm:"column:event_type;not null;type:varchar(255)"`
	Payload   []byte `gorm:"column:payload;not null;type:jsonb"`
}

func (OutboxEvent) TableName() string {
	return "outbox_events"
}
