package model

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
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

// PopulateFromInventory unmarshals raw inventory JSON and extracts
// VCenterID and resource counts into the receiver. It sets Inventory to raw.
func NewAssessmentInventory(id uuid.UUID, name string, raw []byte) (AssessmentInventory, error) {
	ai := AssessmentInventory{
		ID:        id,
		Name:      name,
		Inventory: raw,
	}

	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		return AssessmentInventory{}, fmt.Errorf("failed to unmarshal inventory: %w", err)
	}

	// NOTE: As we backported the snapshot table content into assessment_inventories, it could happen
	// that a v1 inventory is copied into the new assessment_inventories table.
	// Therefore, we need to check of version
	_, hasClusters := keys["clusters"]
	_, hasVCenterID := keys["vcenter_id"]
	if hasClusters || hasVCenterID {
		ai.Version = SnapshotVersionV2
		var inv v1alpha1.Inventory
		if err := json.Unmarshal(raw, &inv); err != nil {
			return AssessmentInventory{}, fmt.Errorf("failed to unmarshal v2 inventory: %w", err)
		}
		ai.VCenterID = inv.VcenterId
		if inv.Vcenter != nil {
			ai.VMsCount = inv.Vcenter.Vms.Total
			ai.HostsCount = inv.Vcenter.Infra.TotalHosts
			ai.NetworksCount = len(inv.Vcenter.Infra.Networks)
			ai.DatastoresCount = len(inv.Vcenter.Infra.Datastores)
		}
	} else {
		ai.Version = SnapshotVersionV1
		var invData v1alpha1.InventoryData
		if err := json.Unmarshal(raw, &invData); err != nil {
			return AssessmentInventory{}, fmt.Errorf("failed to unmarshal v1 inventory: %w", err)
		}
		ai.VMsCount = invData.Vms.Total
		ai.HostsCount = invData.Infra.TotalHosts
		ai.NetworksCount = len(invData.Infra.Networks)
		ai.DatastoresCount = len(invData.Infra.Datastores)
		if invData.Vcenter != nil {
			ai.VCenterID = invData.Vcenter.Id
		}
	}
	return ai, nil
}
