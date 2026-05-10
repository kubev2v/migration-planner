package model

import (
	"time"

	"github.com/google/uuid"
)

type RVToolsFile struct {
	ID        uuid.UUID `gorm:"primaryKey;column:id;type:uuid"`
	Data      []byte    `gorm:"column:data;type:bytea;not null"`
	CreatedAt time.Time `gorm:"column:created_at;not null;default:now()"`
}

func (RVToolsFile) TableName() string {
	return "rvtools_files"
}
