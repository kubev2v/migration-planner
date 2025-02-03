package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"gorm.io/gorm"
)

type Source struct {
	ID         uuid.UUID `gorm:"primaryKey; not null"`
	Name       string    `gorm:"not null"`
	Username   string
	OrgID      string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt
	Inventory  *JSONField[api.Inventory] `gorm:"type:jsonb"`
	OnPremises bool
	Agent      *Agent
}

type SourceList []Source

func (s Source) String() string {
	val, _ := json.Marshal(s)
	return string(val)
}
