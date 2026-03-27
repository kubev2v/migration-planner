package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Group struct {
	ID          uuid.UUID `gorm:"primaryKey;column:id;type:VARCHAR(255);"`
	CreatedAt   time.Time `gorm:"not null;default:now()"`
	UpdatedAt   *time.Time
	Name        string `gorm:"uniqueIndex:idx_company_name;not null"`
	Description string
	Kind        string     `gorm:"not null;type:VARCHAR(50)"`
	Icon        string     `gorm:"not null;type:VARCHAR"`
	Company     string     `gorm:"uniqueIndex:idx_company_name;not null;type:VARCHAR(200)"`
	ParentID    *uuid.UUID `gorm:"type:VARCHAR(255)"`
	Members     []Member   `gorm:"foreignKey:GroupID;references:ID;"`
}

type GroupList []Group

func (g Group) String() string {
	val, _ := json.Marshal(g)
	return string(val)
}
