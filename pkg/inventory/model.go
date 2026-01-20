package inventory

// Inventory is the domain representation of infrastructure inventory.
// It contains vCenter-level data and per-cluster inventories.
type Inventory struct {
	VCenterID string
	VCenter   *InventoryData
	Clusters  map[string]InventoryData
}

// InventoryData contains VM and infrastructure data for a scope (vCenter or cluster).
type InventoryData struct {
	VMs   VMsData
	Infra InfraData
}

// VMsData contains aggregated VM statistics and distribution data.
type VMsData struct {
	Total                       int
	TotalMigratable             int
	TotalMigratableWithWarnings int
	PowerStates                 map[string]int
	OSInfo                      map[string]OSInfo
	CPUCores                    ResourceBreakdown
	RamGB                       ResourceBreakdown
	DiskCount                   ResourceBreakdown
	DiskGB                      ResourceBreakdown
	NicCount                    ResourceBreakdown
	DistributionByCPUTier       map[string]int
	DistributionByMemoryTier    map[string]int
	DistributionByNICCount      map[string]int
	DiskSizeTiers               map[string]DiskSizeTierSummary
	DiskTypes                   map[string]DiskTypeSummary
	MigrationWarnings           []MigrationIssue
	NotMigratableReasons        []MigrationIssue
}

// InfraData contains infrastructure-level data (hosts, datastores, networks).
type InfraData struct {
	Hosts                 []Host
	HostPowerStates       map[string]int
	Datastores            []Datastore
	Networks              []Network
	TotalHosts            int
	TotalDatacenters      int
	ClustersPerDatacenter []int
	CPUOverCommitment     *float64
	MemoryOverCommitment  *float64
}

// ResourceBreakdown contains resource totals split by migrability status.
type ResourceBreakdown struct {
	Total                          int
	TotalForMigratable             int
	TotalForMigratableWithWarnings int
	TotalForNotMigratable          int
}

// OSInfo contains OS distribution information.
type OSInfo struct {
	Count                 int
	IsSupported           bool
	UpgradeRecommendation string
}

// MigrationIssue represents a migration concern with its count.
type MigrationIssue struct {
	ID         string
	Label      string
	Category   string
	Assessment string
	Count      int
}

// DiskSizeTierSummary contains VM count and total size for a disk size tier.
type DiskSizeTierSummary struct {
	VMCount     int
	TotalSizeTB float64
}

// DiskTypeSummary contains disk usage aggregated by datastore type.
type DiskTypeSummary struct {
	Type        string
	VMCount     int
	TotalSizeTB float64
}

// Host represents a VMware ESXi host.
type Host struct {
	ID         string
	Vendor     string
	Model      string
	CpuCores   int
	CpuSockets int
	MemoryMB   int
}

// Datastore represents a VMware datastore.
type Datastore struct {
	DiskId          string
	FreeCapacityGB  float64
	TotalCapacityGB float64
	Type            string
	HostId          string
	Model           string
	ProtocolType    string
	Vendor          string
}

// Network represents a VMware network.
type Network struct {
	Name     string
	Type     string
	Dvswitch string
	VlanId   string
	VmsCount int
}
