package model

import (
	"time"

	"github.com/google/uuid"
)

// Assessment is the service-layer domain representation of an assessment
type Assessment struct {
	ID                uuid.UUID
	CreatedAt         time.Time
	UpdatedAt         *time.Time
	Name              string
	OrgID             string
	Username          string
	OwnerFirstName    *string
	OwnerLastName     *string
	SourceType        string
	SourceID          *uuid.UUID
	MainInventory     AssessmentInventory
	SubsetInventories []AssessmentInventory
	Permissions       []Permission
	Sharing           *Sharing
}

// AssessmentSummary is a lightweight projection of an assessment used by list
// endpoints.
type AssessmentSummary struct {
	ID             uuid.UUID
	CreatedAt      time.Time
	Name           string
	OrgID          string
	Username       string
	OwnerFirstName *string
	OwnerLastName  *string
	SourceType     string
	SourceID       *uuid.UUID

	VMsCount        int
	HostsCount      int
	NetworksCount   int
	DatastoresCount int

	Permissions []Permission
	Sharing     *Sharing
}

// AssessmentList contains the lightweight assessment summaries returned by the store.
type AssessmentList []AssessmentSummary

// AssessmentInventory is the service-layer domain representation of a stored
// inventory snapshot. Unlike the persistence model, Inventory is a structured
// domain type rather than a raw jsonb blob.
type AssessmentInventory struct {
	ID              uuid.UUID
	CreatedAt       time.Time
	Name            string
	IsSubset        bool
	VCenterID       string
	VMsCount        int
	HostsCount      int
	NetworksCount   int
	DatastoresCount int
	Version         uint
	Inventory       Inventory
	AssessmentID    uuid.UUID
}

// NewAssessmentInventory builds a snapshot and derives its resource counts.
func NewAssessmentInventory(id, assessmentID uuid.UUID, name string, version uint, isSubset bool, inventory Inventory) AssessmentInventory {
	result := AssessmentInventory{
		ID: id, AssessmentID: assessmentID, Name: name, IsSubset: isSubset,
		Version: version, VCenterID: inventory.VcenterId, Inventory: inventory,
	}
	if inventory.Vcenter != nil {
		result.VMsCount = inventory.Vcenter.Vms.Total
		result.HostsCount = inventory.Vcenter.Infra.TotalHosts
		result.NetworksCount = len(inventory.Vcenter.Infra.Networks)
		result.DatastoresCount = len(inventory.Vcenter.Infra.Datastores)
	}
	return result
}
