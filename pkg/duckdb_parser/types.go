package duckdb_parser

// Filters for querying data.
type Filters struct {
	Cluster    string // filter by cluster name
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

// DiskTypeSummary contains disk usage aggregated by datastore type.
type DiskTypeSummary struct {
	Type        string
	VMCount     int
	TotalSizeTB float64
}

// DiskSizeTierSummary contains VM count and total size for a disk size tier.
type DiskSizeTierSummary struct {
	VMCount     int
	TotalSizeTB float64
}

// MigrationIssue represents an aggregated migration issue.
type MigrationIssue struct {
	ID         string
	Label      string
	Category   string
	Assessment string
	Count      int
}

// VMResourceBreakdown contains resource totals split by migrability status.
type VMResourceBreakdown struct {
	Total                          int
	TotalForMigratable             int
	TotalForMigratableWithWarnings int
	TotalForNotMigratable          int
}

// AllResourceBreakdowns holds all resource breakdowns by migrability.
type AllResourceBreakdowns struct {
	CpuCores  VMResourceBreakdown
	RamGB     VMResourceBreakdown
	DiskCount VMResourceBreakdown
	DiskGB    VMResourceBreakdown
	NicCount  VMResourceBreakdown
}
