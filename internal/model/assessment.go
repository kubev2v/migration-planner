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
