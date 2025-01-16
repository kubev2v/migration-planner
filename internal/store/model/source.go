package model

import (
	"encoding/json"
	"time"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"gorm.io/gorm"
)

type Source struct {
	ID         openapi_types.UUID `json:"id" gorm:"primaryKey"`
	Username   string
	OrgID      string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt            `gorm:"index"`
	Inventory  *JSONField[api.Inventory] `gorm:"type:jsonb"`
	OnPremises bool
	Agents     []Agent `gorm:"constraint:OnDelete:SET NULL;"`
}

type SourceList []Source

func (s Source) String() string {
	val, _ := json.Marshal(s)
	return string(val)
}
