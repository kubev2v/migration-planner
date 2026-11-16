package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type AssessmentInventory struct {
	ID              uuid.UUID `gorm:"primaryKey;type:TEXT"`
	CreatedAt       time.Time `gorm:"not null;default:now()"`
	Name            string    `gorm:"not null"`
	IsSubset        bool      `gorm:"not null;default:false"`
	VCenterID       string    `gorm:"column:vcenter_id;not null"`
	VMsCount        int       `gorm:"column:vms_count;not null"`
	HostsCount      int       `gorm:"column:hosts_count;not null"`
	NetworksCount   int       `gorm:"column:networks_count;not null"`
	DatastoresCount int       `gorm:"column:datastores_count;not null"`
	Version         uint      `gorm:"type:smallint;not null;default:1"`
	Inventory       []byte    `gorm:"type:jsonb;not null"`
	AssessmentID    uuid.UUID `gorm:"type:VARCHAR(255);not null;index"`
}

func (AssessmentInventory) TableName() string {
	return "assessment_inventories"
}

type AssessmentInventoryList []AssessmentInventory

func (ai AssessmentInventory) String() string {
	val, _ := json.Marshal(ai)
	return string(val)
}
