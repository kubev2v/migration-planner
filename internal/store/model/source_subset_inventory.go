package model

import (
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SourceSubsetInventory struct {
	gorm.Model
	ID         uuid.UUID `gorm:"primaryKey;"`
	Name       string    `gorm:"not null"`
	SourceID   uuid.UUID `gorm:"column:source_id;not null;index"`
	VCenterID  string    `gorm:"column:v_center_id"`
	VMsCount   int       `gorm:"column:vms_count"`
	Inventory  []byte    `gorm:"type:jsonb;not null"`
	UpdateType string    `gorm:"type:VARCHAR(10);default:auto"`
}

func (SourceSubsetInventory) TableName() string {
	return "source_subset_inventories"
}

type SourceSubsetInventoryList []SourceSubsetInventory

func (si SourceSubsetInventory) String() string {
	val, _ := json.Marshal(si)
	return string(val)
}
