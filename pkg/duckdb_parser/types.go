package duckdb_parser

import "github.com/kubev2v/migration-planner/pkg/inventory"

// Filters for querying data.
type Filters struct {
	Cluster    string // filter by cluster name
	VmId       string // filter by vm ID
	OS         string // filter by OS name (for VMs)
	PowerState string // filter by power state (for VMs)
}

// Options for pagination.
type Options struct {
	Limit  int // max results (0 = unlimited)
	Offset int // skip first N results
}

// ResourceTotals contains aggregated resource totals.
type ResourceTotals struct {
	TotalCPUCores  int
	TotalRAMGB     int
	TotalDiskCount int
	TotalDiskGB    int
	TotalNICCount  int
}

// AllResourceBreakdowns holds all resource breakdowns by migrability.
// Uses inventory.ResourceBreakdown to avoid type duplication.
type AllResourceBreakdowns struct {
	CpuCores  inventory.ResourceBreakdown
	RamGB     inventory.ResourceBreakdown
	DiskCount inventory.ResourceBreakdown
	DiskGB    inventory.ResourceBreakdown
	NicCount  inventory.ResourceBreakdown
}
