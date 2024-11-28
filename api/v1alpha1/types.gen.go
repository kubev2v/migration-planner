// Package v1alpha1 provides primitives to interact with the openapi HTTP API.
//
// Code generated by github.com/oapi-codegen/oapi-codegen/v2 version v2.3.0 DO NOT EDIT.
package v1alpha1

import (
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
)

// Defines values for AgentStatus.
const (
	AgentStatusError                     AgentStatus = "error"
	AgentStatusGatheringInitialInventory AgentStatus = "gathering-initial-inventory"
	AgentStatusNotConnected              AgentStatus = "not-connected"
	AgentStatusSourceGone                AgentStatus = "source-gone"
	AgentStatusUpToDate                  AgentStatus = "up-to-date"
	AgentStatusWaitingForCredentials     AgentStatus = "waiting-for-credentials"
)

// Defines values for InfraNetworksType.
const (
	Distributed InfraNetworksType = "distributed"
	Dvswitch    InfraNetworksType = "dvswitch"
	Standard    InfraNetworksType = "standard"
	Unsupported InfraNetworksType = "unsupported"
)

// Defines values for SourceStatus.
const (
	SourceStatusError                     SourceStatus = "error"
	SourceStatusGatheringInitialInventory SourceStatus = "gathering-initial-inventory"
	SourceStatusNotConnected              SourceStatus = "not-connected"
	SourceStatusUpToDate                  SourceStatus = "up-to-date"
	SourceStatusWaitingForCredentials     SourceStatus = "waiting-for-credentials"
)

// Agent defines model for Agent.
type Agent struct {
	Associated    bool        `json:"associated"`
	CreatedAt     time.Time   `json:"createdAt"`
	CredentialUrl string      `json:"credentialUrl"`
	DeletedAt     *time.Time  `json:"deletedAt,omitempty"`
	Id            string      `json:"id"`
	SourceId      *string     `json:"sourceId,omitempty"`
	Status        AgentStatus `json:"status"`
	StatusInfo    string      `json:"statusInfo"`
	UpdatedAt     time.Time   `json:"updatedAt"`
	Version       string      `json:"version"`
}

// AgentStatus defines model for Agent.Status.
type AgentStatus string

// AgentList defines model for AgentList.
type AgentList = []Agent

// Error defines model for Error.
type Error struct {
	// Message Error message
	Message string `json:"message"`
}

// Infra defines model for Infra.
type Infra struct {
	Datastores []struct {
		FreeCapacityGB  int    `json:"freeCapacityGB"`
		TotalCapacityGB int    `json:"totalCapacityGB"`
		Type            string `json:"type"`
	} `json:"datastores"`
	HostPowerStates map[string]int `json:"hostPowerStates"`
	HostsPerCluster []int          `json:"hostsPerCluster"`
	Networks        []struct {
		Name   string            `json:"name"`
		Type   InfraNetworksType `json:"type"`
		VlanId *string           `json:"vlanId,omitempty"`
	} `json:"networks"`
	TotalClusters int `json:"totalClusters"`
	TotalHosts    int `json:"totalHosts"`
}

// InfraNetworksType defines model for Infra.Networks.Type.
type InfraNetworksType string

// Inventory defines model for Inventory.
type Inventory struct {
	Infra   Infra   `json:"infra"`
	Vcenter VCenter `json:"vcenter"`
	Vms     VMs     `json:"vms"`
}

// MigrationIssues defines model for MigrationIssues.
type MigrationIssues = []struct {
	Assessment string `json:"assessment"`
	Count      int    `json:"count"`
	Label      string `json:"label"`
}

// Source defines model for Source.
type Source struct {
	Agents     *[]SourceAgentItem `json:"agents,omitempty"`
	CreatedAt  time.Time          `json:"createdAt"`
	Id         openapi_types.UUID `json:"id"`
	Inventory  *Inventory         `json:"inventory,omitempty"`
	Name       string             `json:"name"`
	SshKey     *string            `json:"sshKey,omitempty"`
	Status     SourceStatus       `json:"status"`
	StatusInfo string             `json:"statusInfo"`
	UpdatedAt  time.Time          `json:"updatedAt"`
}

// SourceStatus defines model for Source.Status.
type SourceStatus string

// SourceAgentItem defines model for SourceAgentItem.
type SourceAgentItem struct {
	Associated bool               `json:"associated"`
	Id         openapi_types.UUID `json:"id"`
}

// SourceCreate defines model for SourceCreate.
type SourceCreate struct {
	Name string `json:"name"`
}

// SourceList defines model for SourceList.
type SourceList = []Source

// Status Status is a return value for calls that don't return other objects.
type Status struct {
	// Message A human-readable description of the status of this operation.
	Message *string `json:"message,omitempty"`

	// Reason A machine-readable description of why this operation is in the "Failure" status. If this value is empty there is no information available. A Reason clarifies an HTTP status code but does not override it.
	Reason *string `json:"reason,omitempty"`

	// Status Status of the operation. One of: "Success" or "Failure". More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	Status *string `json:"status,omitempty"`
}

// VCenter defines model for VCenter.
type VCenter struct {
	Id string `json:"id"`
}

// VMResourceBreakdown defines model for VMResourceBreakdown.
type VMResourceBreakdown struct {
	Histogram struct {
		Data     []int `json:"data"`
		MinValue int   `json:"minValue"`
		Step     int   `json:"step"`
	} `json:"histogram"`
	Total                          int `json:"total"`
	TotalForMigratable             int `json:"totalForMigratable"`
	TotalForMigratableWithWarnings int `json:"totalForMigratableWithWarnings"`
	TotalForNotMigratable          int `json:"totalForNotMigratable"`
}

// VMs defines model for VMs.
type VMs struct {
	CpuCores                    VMResourceBreakdown `json:"cpuCores"`
	DiskCount                   VMResourceBreakdown `json:"diskCount"`
	DiskGB                      VMResourceBreakdown `json:"diskGB"`
	MigrationWarnings           MigrationIssues     `json:"migrationWarnings"`
	NotMigratableReasons        MigrationIssues     `json:"notMigratableReasons"`
	Os                          map[string]int      `json:"os"`
	PowerStates                 map[string]int      `json:"powerStates"`
	RamGB                       VMResourceBreakdown `json:"ramGB"`
	Total                       int                 `json:"total"`
	TotalMigratable             int                 `json:"totalMigratable"`
	TotalMigratableWithWarnings *int                `json:"totalMigratableWithWarnings,omitempty"`
}

// GetImageParams defines parameters for GetImage.
type GetImageParams struct {
	// SshKey public SSH key
	SshKey *string `form:"sshKey,omitempty" json:"sshKey,omitempty"`
}

// HeadImageParams defines parameters for HeadImage.
type HeadImageParams struct {
	// SshKey public SSH key
	SshKey *string `form:"sshKey,omitempty" json:"sshKey,omitempty"`
}
