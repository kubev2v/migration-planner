package model

import (
	"encoding/json"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"gorm.io/gorm"
)

type Label struct {
	Key      string `gorm:"primaryKey;column:key;type:VARCHAR;size:100"`
	Value    string `gorm:"column:value;type:VARCHAR;size:100"`
	SourceID string `gorm:"primaryKey;column:source_id;type:TEXT"`
}

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
	Labels     []Label
}

type SourceList []Source

func (s Source) String() string {
	val, _ := json.Marshal(s)
	return string(val)
}
