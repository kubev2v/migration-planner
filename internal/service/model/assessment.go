package model

import (
	"encoding/json"
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

// AssessmentList is a slice of domain assessments.
type AssessmentList []Assessment

func (a Assessment) String() string {
	val, _ := json.Marshal(a)
	return string(val)
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

	// Inventories are populated only when requested via
	// AssessmentFilter.WithInventories. MainInventory is otherwise zero-valued.
	MainInventory     AssessmentInventory
	SubsetInventories []AssessmentInventory
}

// AssessmentSummaryList is a slice of domain assessment summaries.
type AssessmentSummaryList []AssessmentSummary

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

// AssessmentInventoryList is a slice of domain assessment inventories.
type AssessmentInventoryList []AssessmentInventory

func (ai AssessmentInventory) String() string {
	val, _ := json.Marshal(ai)
	return string(val)
}
