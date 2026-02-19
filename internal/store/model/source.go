package model

import (
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Label struct {
	Key      string `gorm:"primaryKey;column:key;type:VARCHAR;size:100;"`
	Value    string `gorm:"column:value;type:VARCHAR;size:100;not null"`
	SourceID string `gorm:"primaryKey;column:source_id;type:TEXT;"`
}

type Source struct {
	gorm.Model
	ID          uuid.UUID `gorm:"primaryKey;"`
	Name        string    `gorm:"uniqueIndex:sources_org_id_user_name;not null"`
	VCenterID   string
	Username    string `gorm:"uniqueIndex:sources_org_id_user_name"`
	OrgID       string `gorm:"uniqueIndex:sources_org_id_user_name;not null"`
	Inventory   []byte `gorm:"type:jsonb"`
	OnPremises  bool
	Agents      []Agent    `gorm:"constraint:OnDelete:CASCADE;"`
	ImageInfra  ImageInfra `gorm:"constraint:OnDelete:CASCADE;"`
	Labels      []Label    `gorm:"foreignKey:SourceID;references:ID;constraint:OnDelete:CASCADE;"`
	EmailDomain *string
}

type SourceList []Source

func (s Source) String() string {
	val, _ := json.Marshal(s)
	return string(val)
}
