package duckdb_parser

import (
	"context"
	"fmt"

	"github.com/georgysavva/scany/v2/sqlscan"

	"github.com/kubev2v/migration-planner/pkg/duckdb_parser/models"
)

// VMs returns VMs with optional filters and pagination.
func (p *Parser) VMs(ctx context.Context, filters Filters, options Options) ([]models.VM, error) {
	q, err := p.builder.VMQuery(filters, options)
	if err != nil {
		return nil, fmt.Errorf("failed to build vm query: %v", err)
	}
	return p.readVMs(ctx, q)
}

// VMCount returns the count of VMs with optional filters.
func (p *Parser) VMCount(ctx context.Context, filters Filters) (int, error) {
	q, err := p.builder.VMCountQuery(filters)
	if err != nil {
		return 0, fmt.Errorf("building vm count query: %w", err)
	}
	var count int
	if err := p.db.QueryRowContext(ctx, q).Scan(&count); err != nil {
		return 0, fmt.Errorf("scanning vm count: %w", err)
	}
	return count, nil
}

// Datastores returns datastores with optional filters and pagination.
func (p *Parser) Datastores(ctx context.Context, filters Filters, options Options) ([]models.Datastore, error) {
	q, err := p.builder.DatastoreQuery(filters, options)
	if err != nil {
		return nil, fmt.Errorf("building datastore query: %w", err)
	}
	var results []models.Datastore
	if err := sqlscan.Select(ctx, p.db, &results, q); err != nil {
		return nil, fmt.Errorf("scanning datastores: %w", err)
	}
	return results, nil
}

// Networks returns networks with optional filters and pagination.
func (p *Parser) Networks(ctx context.Context, filters Filters, options Options) ([]models.Network, error) {
	q, err := p.builder.NetworkQuery(filters, options)
	if err != nil {
		return nil, fmt.Errorf("building network query: %w", err)
	}
	var results []models.Network
	if err := sqlscan.Select(ctx, p.db, &results, q); err != nil {
		return nil, fmt.Errorf("scanning networks: %w", err)
	}
	return results, nil
}

// Hosts returns hosts with optional filters and pagination.
func (p *Parser) Hosts(ctx context.Context, filters Filters, options Options) ([]models.Host, error) {
	q, err := p.builder.HostQuery(filters, options)
	if err != nil {
		return nil, fmt.Errorf("building host query: %w", err)
	}
	var results []models.Host
	if err := sqlscan.Select(ctx, p.db, &results, q); err != nil {
		return nil, fmt.Errorf("scanning hosts: %w", err)
	}
	return results, nil
}

// Clusters returns a list of unique cluster names.
func (p *Parser) Clusters(ctx context.Context) ([]string, error) {
	q, err := p.builder.ClustersQuery()
	if err != nil {
		return nil, fmt.Errorf("building clusters query: %w", err)
	}
	var clusters []string
	rows, err := p.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("querying clusters: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cluster string
		if err := rows.Scan(&cluster); err != nil {
			return nil, fmt.Errorf("scanning cluster: %w", err)
		}
		clusters = append(clusters, cluster)
	}
	return clusters, rows.Err()
}

// OsSummary returns OS distribution with optional filters.
func (p *Parser) OsSummary(ctx context.Context, filters Filters) ([]models.Os, error) {
	q, err := p.builder.OsQuery(filters)
	if err != nil {
		return nil, fmt.Errorf("building os query: %w", err)
	}
	var results []models.Os
	if err := sqlscan.Select(ctx, p.db, &results, q); err != nil {
		return nil, fmt.Errorf("scanning OS: %w", err)
	}
	return results, nil
}

// PowerStateCounts returns VM power state distribution.
func (p *Parser) PowerStateCounts(ctx context.Context, filters Filters) (map[string]int, error) {
	q, err := p.builder.PowerStateCountsQuery(filters)
	if err != nil {
		return nil, fmt.Errorf("building power state counts query: %w", err)
	}
	return p.readStringIntMap(ctx, q)
}

// HostPowerStateCounts returns host power state distribution.
func (p *Parser) HostPowerStateCounts(ctx context.Context, filters Filters) (map[string]int, error) {
	q, err := p.builder.HostPowerStateCountsQuery(filters)
	if err != nil {
		return nil, fmt.Errorf("building host power state counts query: %w", err)
	}
	return p.readStringIntMap(ctx, q)
}

// VMCountByNetwork returns VM count per network.
func (p *Parser) VMCountByNetwork(ctx context.Context, filters Filters) (map[string]int, error) {
	q, err := p.builder.VMCountByNetworkQuery(filters)
	if err != nil {
		return nil, fmt.Errorf("building vm count by network query: %w", err)
	}
	return p.readStringIntMap(ctx, q)
}

// CPUTierDistribution returns VM distribution by CPU tier.
func (p *Parser) CPUTierDistribution(ctx context.Context, filters Filters) (map[string]int, error) {
	q, err := p.builder.CPUTierQuery(filters)
	if err != nil {
		return nil, fmt.Errorf("building cpu tier query: %w", err)
	}
	return p.readStringIntMap(ctx, q)
}

// MemoryTierDistribution returns VM distribution by memory tier.
func (p *Parser) MemoryTierDistribution(ctx context.Context, filters Filters) (map[string]int, error) {
	q, err := p.builder.MemoryTierQuery(filters)
	if err != nil {
		return nil, fmt.Errorf("building memory tier query: %w", err)
	}
	return p.readStringIntMap(ctx, q)
}

// DiskSizeTierDistribution returns VM distribution by disk size tier.
func (p *Parser) DiskSizeTierDistribution(ctx context.Context, filters Filters) (map[string]DiskSizeTierSummary, error) {
	q, err := p.builder.DiskSizeTierQuery(filters)
	if err != nil {
		return nil, fmt.Errorf("building disk size tier query: %w", err)
	}
	result := make(map[string]DiskSizeTierSummary)
	rows, err := p.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("querying disk size tier: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var tier string
		var summary DiskSizeTierSummary
		if err := rows.Scan(&tier, &summary.VMCount, &summary.TotalSizeTB); err != nil {
			return nil, fmt.Errorf("scanning disk size tier: %w", err)
		}
		result[tier] = summary
	}
	return result, rows.Err()
}

// DiskTypeSummary returns disk usage aggregated by datastore type.
func (p *Parser) DiskTypeSummary(ctx context.Context, filters Filters) ([]DiskTypeSummary, error) {
	q, err := p.builder.DiskTypeSummaryQuery(filters)
	if err != nil {
		return nil, fmt.Errorf("building disk type summary query: %w", err)
	}
	var results []DiskTypeSummary
	rows, err := p.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("querying disk type summary: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var s DiskTypeSummary
		if err := rows.Scan(&s.Type, &s.VMCount, &s.TotalSizeTB); err != nil {
			return nil, fmt.Errorf("scanning disk type summary: %w", err)
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

// TotalResources returns aggregated resource totals.
func (p *Parser) TotalResources(ctx context.Context, filters Filters) (ResourceTotals, error) {
	q, err := p.builder.ResourceTotalsQuery(filters)
	if err != nil {
		return ResourceTotals{}, fmt.Errorf("building resource totals query: %w", err)
	}
	var r ResourceTotals
	if err := p.db.QueryRowContext(ctx, q).Scan(
		&r.TotalCPUCores, &r.TotalRAMGB, &r.TotalDiskCount, &r.TotalDiskGB, &r.TotalNICCount,
	); err != nil {
		return ResourceTotals{}, fmt.Errorf("scanning resource totals: %w", err)
	}
	return r, nil
}

// AllocatedVCPUs returns sum of vCPUs for powered-on VMs.
func (p *Parser) AllocatedVCPUs(ctx context.Context, filters Filters) (int, error) {
	q, err := p.builder.AllocatedVCPUsQuery(filters)
	if err != nil {
		return 0, fmt.Errorf("building allocated vcpus query: %w", err)
	}
	var count int
	if err := p.db.QueryRowContext(ctx, q).Scan(&count); err != nil {
		return 0, fmt.Errorf("scanning allocated vcpus: %w", err)
	}
	return count, nil
}

// AllocatedMemoryMB returns sum of memory (MB) for powered-on VMs.
func (p *Parser) AllocatedMemoryMB(ctx context.Context, filters Filters) (int, error) {
	q, err := p.builder.AllocatedMemoryQuery(filters)
	if err != nil {
		return 0, fmt.Errorf("building allocated memory query: %w", err)
	}
	var count int
	if err := p.db.QueryRowContext(ctx, q).Scan(&count); err != nil {
		return 0, fmt.Errorf("scanning allocated memory: %w", err)
	}
	return count, nil
}

// TotalHostCPUCores returns sum of physical CPU cores across hosts.
func (p *Parser) TotalHostCPUCores(ctx context.Context, filters Filters) (int, error) {
	q, err := p.builder.TotalHostCPUsQuery(filters)
	if err != nil {
		return 0, fmt.Errorf("building total host cpus query: %w", err)
	}
	var count int
	if err := p.db.QueryRowContext(ctx, q).Scan(&count); err != nil {
		return 0, fmt.Errorf("scanning total host cpus: %w", err)
	}
	return count, nil
}

// TotalHostMemoryMB returns sum of host memory (MB).
func (p *Parser) TotalHostMemoryMB(ctx context.Context, filters Filters) (int, error) {
	q, err := p.builder.TotalHostMemoryQuery(filters)
	if err != nil {
		return 0, fmt.Errorf("building total host memory query: %w", err)
	}
	var count int
	if err := p.db.QueryRowContext(ctx, q).Scan(&count); err != nil {
		return 0, fmt.Errorf("scanning total host memory: %w", err)
	}
	return count, nil
}

// MigratableVMCount returns count of VMs without Critical concerns.
func (p *Parser) MigratableVMCount(ctx context.Context, filters Filters) (int, error) {
	q, err := p.builder.MigratableCountQuery(filters)
	if err != nil {
		return 0, fmt.Errorf("building migratable count query: %w", err)
	}
	var count int
	if err := p.db.QueryRowContext(ctx, q).Scan(&count); err != nil {
		return 0, fmt.Errorf("scanning migratable count: %w", err)
	}
	return count, nil
}

// MigratableWithWarningsVMCount returns count of VMs with Warning but no Critical concerns.
func (p *Parser) MigratableWithWarningsVMCount(ctx context.Context, filters Filters) (int, error) {
	q, err := p.builder.MigratableWithWarningsCountQuery(filters)
	if err != nil {
		return 0, fmt.Errorf("building migratable with warnings count query: %w", err)
	}
	var count int
	if err := p.db.QueryRowContext(ctx, q).Scan(&count); err != nil {
		return 0, fmt.Errorf("scanning migratable with warnings count: %w", err)
	}
	return count, nil
}

// NotMigratableVMCount returns count of VMs with Critical concerns.
func (p *Parser) NotMigratableVMCount(ctx context.Context, filters Filters) (int, error) {
	q, err := p.builder.NotMigratableCountQuery(filters)
	if err != nil {
		return 0, fmt.Errorf("building not migratable count query: %w", err)
	}
	var count int
	if err := p.db.QueryRowContext(ctx, q).Scan(&count); err != nil {
		return 0, fmt.Errorf("scanning not migratable count: %w", err)
	}
	return count, nil
}

// MigrationIssues returns aggregated migration issues by category.
func (p *Parser) MigrationIssues(ctx context.Context, filters Filters, category string) ([]MigrationIssue, error) {
	q, err := p.builder.MigrationIssuesQuery(filters, category)
	if err != nil {
		return nil, fmt.Errorf("building migration issues query: %w", err)
	}
	var results []MigrationIssue
	rows, err := p.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("querying migration issues: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var m MigrationIssue
		if err := rows.Scan(&m.ID, &m.Label, &m.Category, &m.Assessment, &m.Count); err != nil {
			return nil, fmt.Errorf("scanning migration issue: %w", err)
		}
		results = append(results, m)
	}
	return results, rows.Err()
}

// ResourceBreakdowns returns all resource breakdowns by migrability status.
func (p *Parser) ResourceBreakdowns(ctx context.Context, filters Filters) (AllResourceBreakdowns, error) {
	q, err := p.builder.ResourceBreakdownsQuery(filters)
	if err != nil {
		return AllResourceBreakdowns{}, fmt.Errorf("building resource breakdowns query: %w", err)
	}
	var result AllResourceBreakdowns
	if err := p.db.QueryRowContext(ctx, q).Scan(
		&result.CpuCores.Total, &result.CpuCores.TotalForMigratable,
		&result.CpuCores.TotalForMigratableWithWarnings, &result.CpuCores.TotalForNotMigratable,
		&result.RamGB.Total, &result.RamGB.TotalForMigratable,
		&result.RamGB.TotalForMigratableWithWarnings, &result.RamGB.TotalForNotMigratable,
		&result.DiskCount.Total, &result.DiskCount.TotalForMigratable,
		&result.DiskCount.TotalForMigratableWithWarnings, &result.DiskCount.TotalForNotMigratable,
		&result.DiskGB.Total, &result.DiskGB.TotalForMigratable,
		&result.DiskGB.TotalForMigratableWithWarnings, &result.DiskGB.TotalForNotMigratable,
		&result.NicCount.Total, &result.NicCount.TotalForMigratable,
		&result.NicCount.TotalForMigratableWithWarnings, &result.NicCount.TotalForNotMigratable,
	); err != nil {
		return AllResourceBreakdowns{}, fmt.Errorf("scanning resource breakdowns: %w", err)
	}
	return result, nil
}

// VCenterID returns the vCenter UUID.
func (p *Parser) VCenterID(ctx context.Context) (string, error) {
	q, err := p.builder.VCenterQuery()
	if err != nil {
		return "", fmt.Errorf("building vcenter query: %w", err)
	}
	var vcenterID string
	if err := p.db.QueryRowContext(ctx, q).Scan(&vcenterID); err != nil {
		return "", fmt.Errorf("scanning vcenter id: %w", err)
	}
	return vcenterID, nil
}

// DatacenterCount returns count of unique datacenters.
func (p *Parser) DatacenterCount(ctx context.Context) (int, error) {
	q, err := p.builder.DatacenterCountQuery()
	if err != nil {
		return 0, fmt.Errorf("building datacenter count query: %w", err)
	}
	var count int
	if err := p.db.QueryRowContext(ctx, q).Scan(&count); err != nil {
		return 0, fmt.Errorf("scanning datacenter count: %w", err)
	}
	return count, nil
}

// VMsWithSharedDisksCount returns count of VMs that have at least one shared disk.
func (p *Parser) VMsWithSharedDisksCount(ctx context.Context, filters Filters) (int, error) {
	q, err := p.builder.VMsWithSharedDisksCountQuery(filters)
	if err != nil {
		return 0, fmt.Errorf("building vms with shared disks count query: %w", err)
	}
	var count int
	if err := p.db.QueryRowContext(ctx, q).Scan(&count); err != nil {
		return 0, fmt.Errorf("scanning vms with shared disks count: %w", err)
	}
	return count, nil
}

// ClustersPerDatacenter returns cluster count per datacenter.
func (p *Parser) ClustersPerDatacenter(ctx context.Context) ([]int, error) {
	q, err := p.builder.ClustersPerDatacenterQuery()
	if err != nil {
		return nil, fmt.Errorf("building clusters per datacenter query: %w", err)
	}
	var counts []int
	rows, err := p.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("querying clusters per datacenter: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var dc string
		var count int
		if err := rows.Scan(&dc, &count); err != nil {
			return nil, fmt.Errorf("scanning clusters per datacenter: %w", err)
		}
		counts = append(counts, count)
	}
	return counts, rows.Err()
}

// readStringIntMap is a helper for reading (string, int) result sets into a map.
func (p *Parser) readStringIntMap(ctx context.Context, query string) (map[string]int, error) {
	result := make(map[string]int)
	rows, err := p.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var count int
		if err := rows.Scan(&key, &count); err != nil {
			return nil, fmt.Errorf("scanning: %w", err)
		}
		result[key] = count
	}
	return result, rows.Err()
}

// readVMs reads VMs from a query result with manual scanning for complex nested types.
func (p *Parser) readVMs(ctx context.Context, query string) ([]models.VM, error) {
	rows, err := p.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying VMs: %w", err)
	}
	defer rows.Close()

	var vms []models.VM
	for rows.Next() {
		var vm models.VM
		if err := rows.Scan(
			&vm.ID,
			&vm.Name,
			&vm.Folder,
			&vm.Host,
			&vm.UUID,
			&vm.Firmware,
			&vm.PowerState,
			&vm.ConnectionState,
			&vm.FaultToleranceEnabled,
			&vm.CpuCount,
			&vm.MemoryMB,
			&vm.GuestName,
			&vm.GuestNameFromVmwareTools,
			&vm.HostName,
			&vm.IpAddress,
			&vm.StorageUsed,
			&vm.IsTemplate,
			&vm.ChangeTrackingEnabled,
			&vm.DiskEnableUuid,
			&vm.Datacenter,
			&vm.Cluster,
			&vm.HWVersion,
			&vm.TotalDiskCapacityMiB,
			&vm.ProvisionedMiB,
			&vm.ResourcePool,
			&vm.CpuHotAddEnabled,
			&vm.CpuHotRemoveEnabled,
			&vm.CpuSockets,
			&vm.CoresPerSocket,
			&vm.MemoryHotAddEnabled,
			&vm.BalloonedMemory,
			&vm.Disks,
			&vm.NICs,
			&vm.Networks,
			&vm.Concerns,
		); err != nil {
			return nil, fmt.Errorf("scanning VM row: %w", err)
		}
		vms = append(vms, vm)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating VM rows: %w", err)
	}
	return vms, nil
}
