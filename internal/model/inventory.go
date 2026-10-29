// Package model defines the service-layer domain models for sources and assessments.
//
// Unlike the persistence model in internal/store/model, the inventory here is a
// fully structured domain type (Inventory) rather than a raw jsonb blob. The
// types mirror the API model in api/v1alpha1 field-for-field (identical JSON
// tags) so the blob stored by the persistence layer can be unmarshaled directly
// into these types, and converted to the API model by the handler mappers.
package model

import "time"

// Inventory versions, mirrored from the persistence layer.
const (
	InventoryVersionV1 = 1 + iota
	InventoryVersionV2
)

const (
	ClusterFeaturesDrsModeFullyAutomated     ClusterFeaturesDrsMode = "Fully Automated"
	ClusterFeaturesDrsModeManual             ClusterFeaturesDrsMode = "Manual"
	ClusterFeaturesDrsModeNone               ClusterFeaturesDrsMode = "None"
	ClusterFeaturesDrsModePartiallyAutomated ClusterFeaturesDrsMode = "Partially Automated"
)

// Inventory is the top-level structured inventory for a source or assessment.
type Inventory struct {
	Clusters       map[string]InventoryData `json:"clusters"`
	CollectionTime *time.Time               `json:"collection_time"`
	CreatedAt      *time.Time               `json:"created_at,omitempty"`
	Vcenter        *InventoryData           `json:"vcenter,omitempty"`
	VcenterId      string                   `json:"vcenter_id"`
	VcenterVersion *string                  `json:"vcenter_version,omitempty"`
}

// InventoryData holds the inventory for a single vCenter or cluster.
type InventoryData struct {
	ClusterFeatures    *ClusterFeatures    `json:"clusterFeatures,omitempty"`
	ClusterUtilization *ClusterUtilization `json:"clusterUtilization,omitempty"`
	Infra              Infra               `json:"infra"`
	Vcenter            *VCenter            `json:"vcenter,omitempty"`
	Vms                VMs                 `json:"vms"`
}

// ClusterFeaturesDrsMode is the DRS automation mode for a cluster.
type ClusterFeaturesDrsMode string

// ClusterFeatures describes cluster-level feature toggles.
type ClusterFeatures struct {
	DrsEnabled *bool                   `json:"drsEnabled,omitempty"`
	DrsMode    *ClusterFeaturesDrsMode `json:"drsMode,omitempty"`
	HaEnabled  *bool                   `json:"haEnabled,omitempty"`
}

// ClusterUtilization captures CPU/memory utilization statistics for a cluster.
type ClusterUtilization struct {
	Confidence float64 `json:"confidence"`
	CpuAvg     float64 `json:"cpu_avg"`
	CpuMax     float64 `json:"cpu_max"`
	CpuP95     float64 `json:"cpu_p95"`
	MemAvg     float64 `json:"mem_avg"`
	MemMax     float64 `json:"mem_max"`
	MemP95     float64 `json:"mem_p95"`
}

// Infra describes the physical/virtual infrastructure of the inventory.
type Infra struct {
	ClustersPerDatacenter *[]int         `json:"clustersPerDatacenter,omitempty"`
	CpuOverCommitment     *float64       `json:"cpuOverCommitment,omitempty"`
	Datastores            []Datastore    `json:"datastores"`
	HostPowerStates       map[string]int `json:"hostPowerStates"`
	Hosts                 *[]Host        `json:"hosts,omitempty"`
	// Deprecated:
	HostsPerCluster         *[]int    `json:"hostsPerCluster,omitempty"`
	MemoryOverCommitment    *float64  `json:"memoryOverCommitment,omitempty"`
	Networks                []Network `json:"networks"`
	StandaloneHostsDetected *bool     `json:"standaloneHostsDetected,omitempty"`
	// Deprecated:
	TotalClusters    *int `json:"totalClusters,omitempty"`
	TotalDatacenters *int `json:"totalDatacenters,omitempty"`
	TotalHosts       int  `json:"totalHosts"`
	// Deprecated:
	VmsPerCluster     *[]int   `json:"vmsPerCluster,omitempty"`
	VmsPerHostAverage *float64 `json:"vmsPerHostAverage,omitempty"`
}

// VCenter identifies a vCenter.
type VCenter struct {
	Id string `json:"id"`
}

// VMs aggregates virtual-machine statistics for an inventory.
type VMs struct {
	ComplexityDistribution *map[string]DiskSizeTierSummary `json:"complexityDistribution,omitempty"`
	CpuCores               VMResourceBreakdown             `json:"cpuCores"`
	DiskComplexityTier     *map[string]DiskSizeTierSummary `json:"diskComplexityTier,omitempty"`
	DiskCount              VMResourceBreakdown             `json:"diskCount"`
	DiskGB                 VMResourceBreakdown             `json:"diskGB"`
	DiskSizeTier           *map[string]DiskSizeTierSummary `json:"diskSizeTier,omitempty"`
	DiskTypes              *map[string]DiskTypeSummary     `json:"diskTypes,omitempty"`
	// Deprecated:
	DistributionByComplexity *map[string]int      `json:"distributionByComplexity,omitempty"`
	DistributionByCpuTier    *map[string]int      `json:"distributionByCpuTier,omitempty"`
	DistributionByMemoryTier *map[string]int      `json:"distributionByMemoryTier,omitempty"`
	DistributionByNicCount   *map[string]int      `json:"distributionByNicCount,omitempty"`
	IssuesBreakdown          *IssuesBreakdown     `json:"issuesBreakdown,omitempty"`
	MigrationWarnings        []MigrationIssue     `json:"migrationWarnings"`
	NicCount                 *VMResourceBreakdown `json:"nicCount,omitempty"`
	NotMigratableReasons     []MigrationIssue     `json:"notMigratableReasons"`
	// Deprecated:
	Os                          *map[string]int     `json:"os,omitempty"`
	OsInfo                      *map[string]OsInfo  `json:"osInfo,omitempty"`
	PowerStates                 map[string]int      `json:"powerStates"`
	RamGB                       VMResourceBreakdown `json:"ramGB"`
	Total                       int                 `json:"total"`
	TotalMigratable             int                 `json:"totalMigratable"`
	TotalMigratableWithWarnings *int                `json:"totalMigratableWithWarnings,omitempty"`
	TotalWithFaultTolerance     *int                `json:"totalWithFaultTolerance,omitempty"`
	TotalWithRDM                *int                `json:"totalWithRDM,omitempty"`
	TotalWithSharedDisks        *int                `json:"totalWithSharedDisks,omitempty"`
}

// DatastoreStorageProtocol is the storage access category derived from a datastore type.
type DatastoreStorageProtocol string

// Datastore describes a single datastore.
type Datastore struct {
	BackingDeviceIds        *[]string                 `json:"backingDeviceIds,omitempty"`
	FilesystemMajorVersion  *int32                    `json:"filesystemMajorVersion,omitempty"`
	FilesystemType          *string                   `json:"filesystemType,omitempty"`
	DiskId                  string                    `json:"diskId"`
	FreeCapacityGB          int                       `json:"freeCapacityGB"`
	HardwareAcceleratedMove bool                      `json:"hardwareAcceleratedMove"`
	HostId                  *string                   `json:"hostId"`
	Model                   string                    `json:"model"`
	ProtocolType            string                    `json:"protocolType"`
	StorageIoConfiguration  *StorageIoConfiguration   `json:"storageIoConfiguration,omitempty"`
	StorageProtocol         *DatastoreStorageProtocol `json:"storageProtocol,omitempty"`
	TotalCapacityGB         int                       `json:"totalCapacityGB"`
	Type                    string                    `json:"type"`
	Vendor                  string                    `json:"vendor"`
	VsanSnapshotSupport     *bool                     `json:"vsanSnapshotSupport,omitempty"`
}

// StorageIoConfigurationCongestionThresholdMode is the congestion threshold mode.
type StorageIoConfigurationCongestionThresholdMode string

// StorageIoConfiguration describes Storage I/O Control settings for a datastore.
type StorageIoConfiguration struct {
	CongestionThreshold     *int                                           `json:"congestionThreshold,omitempty"`
	CongestionThresholdMode *StorageIoConfigurationCongestionThresholdMode `json:"congestionThresholdMode,omitempty"`
	Enabled                 *bool                                          `json:"enabled,omitempty"`
	PercentOfPeakThroughput *int                                           `json:"percentOfPeakThroughput,omitempty"`
}

// Host describes a single ESXi host.
type Host struct {
	CpuCores                *int    `json:"cpuCores"`
	CpuSockets              *int    `json:"cpuSockets"`
	Id                      *string `json:"id,omitempty"`
	MemoryMB                *int64  `json:"memoryMB"`
	Model                   string  `json:"model"`
	StorageVmotionSupported *bool   `json:"storageVmotionSupported,omitempty"`
	Vendor                  string  `json:"vendor"`
	VmotionSupported        *bool   `json:"vmotionSupported,omitempty"`
}

// NetworkType is the type of a network.
type NetworkType string

// Network describes a single network.
type Network struct {
	Dvswitch *string     `json:"dvswitch,omitempty"`
	Name     string      `json:"name"`
	Type     NetworkType `json:"type"`
	VlanId   *string     `json:"vlanId,omitempty"`
	VmsCount *int        `json:"vmsCount,omitempty"`
}

// VMResourceBreakdown breaks a resource total down by migratability.
type VMResourceBreakdown struct {
	// Deprecated:
	Histogram                      *Histogram `json:"histogram,omitempty"`
	Total                          int        `json:"total"`
	TotalForMigratable             int        `json:"totalForMigratable"`
	TotalForMigratableWithWarnings int        `json:"totalForMigratableWithWarnings"`
	TotalForNotMigratable          int        `json:"totalForNotMigratable"`
}

// Histogram is a simple histogram.
type Histogram struct {
	Data     []int `json:"data"`
	MinValue int   `json:"minValue"`
	Step     int   `json:"step"`
}

// DiskSizeTierSummary summarizes a disk-size tier.
type DiskSizeTierSummary struct {
	TotalSizeTB float64 `json:"totalSizeTB"`
	VmCount     int     `json:"vmCount"`
}

// DiskTypeSummary summarizes a disk type.
type DiskTypeSummary struct {
	TotalSizeTB float64 `json:"totalSizeTB"`
	VmCount     int     `json:"vmCount"`
}

// IssuesBreakdown counts VMs by highest-severity issue.
type IssuesBreakdown struct {
	Advisory    int `json:"advisory"`
	Critical    int `json:"critical"`
	Error       int `json:"error"`
	Information int `json:"information"`
	Warning     int `json:"warning"`
}

// MigrationIssue describes a single migration warning or blocker.
type MigrationIssue struct {
	Assessment string  `json:"assessment"`
	Count      int     `json:"count"`
	Id         *string `json:"id,omitempty"`
	Label      string  `json:"label"`
}

// OsInfoSupportTier is the OS support tier.
type OsInfoSupportTier string

// OsInfo describes operating-system distribution information.
type OsInfo struct {
	Count                 int                `json:"count"`
	SupportTier           *OsInfoSupportTier `json:"supportTier,omitempty"`
	Supported             bool               `json:"supported"`
	UpgradeRecommendation *string            `json:"upgradeRecommendation,omitempty"`
}
