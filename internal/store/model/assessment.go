package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	InventoryVersionV1 = 1 + iota
	InventoryVersionV2
)

type Assessment struct {
	ID             uuid.UUID `gorm:"primaryKey;column:id;type:VARCHAR(255);"`
	CreatedAt      time.Time `gorm:"not null;default:now()"`
	UpdatedAt      *time.Time
	Name           string                `gorm:"not null;uniqueIndex:org_id_user_name"`
	OrgID          string                `gorm:"not null;uniqueIndex:org_id_user_name;index:assessments_org_id_idx"`
	Username       string                `gorm:"type:VARCHAR(255);uniqueIndex:org_id_user_name"`
	OwnerFirstName *string               `gorm:"type:VARCHAR(100)"`
	OwnerLastName  *string               `gorm:"type:VARCHAR(100)"`
	SourceType     string                `gorm:"not null;type:VARCHAR(100)"`
	SourceID       *uuid.UUID            `gorm:"type:TEXT"`
	Inventories    []AssessmentInventory `gorm:"foreignKey:AssessmentID;references:ID;constraint:OnDelete:CASCADE;"`
	Permissions    []Permission          `gorm:"-"`
	Sharing        *Sharing              `gorm:"-"`
}

type AssessmentList []Assessment

func (a Assessment) String() string {
	val, _ := json.Marshal(a)
	return string(val)
}
