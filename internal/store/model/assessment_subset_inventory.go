package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type AssessmentSubsetInventory struct {
	ID         uuid.UUID `gorm:"primaryKey;"`
	CreatedAt  time.Time `gorm:"not null;default:now()"`
	Name       string    `gorm:"not null"`
	SnapshotID uint      `gorm:"column:snapshot_id;not null;index"`
	VCenterID  string    `gorm:"column:v_center_id"`
	VMsCount   int       `gorm:"column:vms_count"`
	Inventory  []byte    `gorm:"type:jsonb;not null"`
}

func (AssessmentSubsetInventory) TableName() string {
	return "assessment_subset_inventories"
}

type AssessmentSubsetInventoryList []AssessmentSubsetInventory

func (ai AssessmentSubsetInventory) String() string {
	val, _ := json.Marshal(ai)
	return string(val)
}
