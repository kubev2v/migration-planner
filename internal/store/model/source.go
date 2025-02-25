package model

import (
	"encoding/json"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"gorm.io/gorm"
)

type Source struct {
	gorm.Model
	ID         uuid.UUID `gorm:"primaryKey; not null"`
	Name       string    `gorm:"uniqueIndex:name_org_id;not null"`
	VCenterID  string
	Username   string
	OrgID      string                    `gorm:"uniqueIndex:name_org_id;not null"`
	Inventory  *JSONField[api.Inventory] `gorm:"type:jsonb"`
	OnPremises bool
	Agents     []Agent    `gorm:"constraint:OnDelete:CASCADE;"`
	ImageInfra ImageInfra `gorm:"constraint:OnDelete:CASCADE;"`
}

type SourceList []Source

func (s Source) String() string {
	val, _ := json.Marshal(s)
	return string(val)
}
