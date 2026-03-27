package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Member struct {
	ID        uuid.UUID `gorm:"primaryKey;column:id;type:VARCHAR(255);"`
	CreatedAt time.Time `gorm:"not null;default:now()"`
	UpdatedAt *time.Time
	Username  string    `gorm:"uniqueIndex;not null;type:VARCHAR(255)"`
	Email     string    `gorm:"not null;type:VARCHAR(255)"`
	GroupID   uuid.UUID `gorm:"not null;type:VARCHAR(255)"`
	Group     *Group    `gorm:"foreignKey:GroupID" json:"-"`
}

type MemberList []Member

func (m Member) String() string {
	val, _ := json.Marshal(m)
	return string(val)
}
