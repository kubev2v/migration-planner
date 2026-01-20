package duckdb_parser

import (
	"context"
	"crypto/sha256"
	"fmt"

	"go.uber.org/zap"

	"github.com/kubev2v/migration-planner/internal/util"
	"github.com/kubev2v/migration-planner/pkg/inventory"
)

// BuildInventory constructs domain inventory from parsed data.
// It builds both vcenter-level and per-cluster inventories.
func (p *Parser) BuildInventory(ctx context.Context) (*inventory.Inventory, error) {
	// Get vCenter ID
	vcenterID, err := p.VCenterID(ctx)
	if err != nil {
		zap.S().Named("duckdb_parser").Warnf("Failed to get vCenter ID: %v", err)
		vcenterID = ""
	}

	// Build vcenter-level inventory (no cluster filter)
	vcenterData, err := p.buildInventoryData(ctx, Filters{})
	if err != nil {
		return nil, fmt.Errorf("building vcenter inventory: %w", err)
	}

	// Get list of clusters
	clusters, err := p.Clusters(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting clusters: %w", err)
	}

	// Get cluster name to Object ID mapping from vCluster sheet
	clusterObjectIDs, err := p.ClusterObjectIDs(ctx)
	if err != nil {
		zap.S().Named("duckdb_parser").Warnf("Failed to get cluster object IDs: %v", err)
		clusterObjectIDs = make(map[string]string)
	}
	zap.S().Named("duckdb_parser").Infof("Found %d clusters in vCluster sheet", len(clusterObjectIDs))

	// Get cluster to datacenter mapping for fallback ID generation
	clusterDatacenters, err := p.ClusterDatacenters(ctx)
	if err != nil {
		zap.S().Named("duckdb_parser").Warnf("Failed to get cluster datacenters: %v", err)
		clusterDatacenters = make(map[string]string)
	}

	// Build per-cluster inventories with cluster IDs from vCluster or generated
	clusterInventories := make(map[string]inventory.InventoryData)
	for _, clusterName := range clusters {
		clusterFilters := Filters{Cluster: clusterName}
		clusterInv, err := p.buildInventoryData(ctx, clusterFilters)
		if err != nil {
			zap.S().Named("duckdb_parser").Warnf("Failed to build inventory for cluster %s: %v", clusterName, err)
			continue
		}

		// Use Object ID from vCluster sheet if available, otherwise generate
		clusterID := resolveClusterID(clusterName, clusterObjectIDs, clusterDatacenters, vcenterID)
		clusterInventories[clusterID] = *clusterInv
	}

	return &inventory.Inventory{
		VCenterID: vcenterID,
		VCenter:   vcenterData,
		Clusters:  clusterInventories,
	}, nil
}

// buildInventoryData constructs an InventoryData for a given filter set.
func (p *Parser) buildInventoryData(ctx context.Context, filters Filters) (*inventory.InventoryData, error) {
	// Build VMs section
	vms, err := p.buildVMsData(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("building VMs: %w", err)
	}

	// Build Infra section
	infra, err := p.buildInfraData(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("building infra: %w", err)
	}

	return &inventory.InventoryData{
		VMs:   *vms,
		Infra: *infra,
	}, nil
}

// buildVMsData constructs the VMs section of InventoryData.
func (p *Parser) buildVMsData(ctx context.Context, filters Filters) (*inventory.VMsData, error) {
	// Get total VM count
	total, err := p.VMCount(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("getting VM count: %w", err)
	}

	// Get power state distribution
	powerStates, err := p.PowerStateCounts(ctx, filters)
	if err != nil {
		zap.S().Named("duckdb_parser").Warnf("Failed to get power states: %v", err)
		powerStates = make(map[string]int)
	}

	// Get OS distribution
	osSummary, err := p.OsSummary(ctx, filters)
	if err != nil {
		zap.S().Named("duckdb_parser").Warnf("Failed to get OS summary: %v", err)
	}
	osInfo := make(map[string]inventory.OSInfo)
	for _, os := range osSummary {
		upgradeRec := ""
		if os.UpgradeRecommendation != nil {
			upgradeRec = *os.UpgradeRecommendation
		}
		osInfo[os.Name] = inventory.OSInfo{
			Count:                 os.Count,
			IsSupported:           os.Supported,
			UpgradeRecommendation: upgradeRec,
		}
	}

	// Get migration counts
	migratableCount, err := p.MigratableVMCount(ctx, filters)
	if err != nil {
		zap.S().Named("duckdb_parser").Warnf("Failed to get migratable count: %v", err)
	}

	migratableWithWarningsCount, err := p.MigratableWithWarningsVMCount(ctx, filters)
	if err != nil {
		zap.S().Named("duckdb_parser").Warnf("Failed to get migratable with warnings count: %v", err)
	}

	// Get resource breakdowns
	breakdowns, err := p.ResourceBreakdowns(ctx, filters)
	if err != nil {
		zap.S().Named("duckdb_parser").Warnf("Failed to get resource breakdowns: %v", err)
	}

	// Get CPU tier distribution
	cpuTiers, err := p.CPUTierDistribution(ctx, filters)
	if err != nil {
		zap.S().Named("duckdb_parser").Warnf("Failed to get CPU tier distribution: %v", err)
	}

	// Get memory tier distribution
	memoryTiers, err := p.MemoryTierDistribution(ctx, filters)
	if err != nil {
		zap.S().Named("duckdb_parser").Warnf("Failed to get memory tier distribution: %v", err)
	}

	// Get NIC tier distribution
	nicTiers, err := p.NicTierDistribution(ctx, filters)
	if err != nil {
		zap.S().Named("duckdb_parser").Warnf("Failed to get NIC tier distribution: %v", err)
	}

	// Get disk size tier distribution (returns inventory types directly)
	diskSizeTiers, err := p.DiskSizeTierDistribution(ctx, filters)
	if err != nil {
		zap.S().Named("duckdb_parser").Warnf("Failed to get disk size tier distribution: %v", err)
		diskSizeTiers = make(map[string]inventory.DiskSizeTierSummary)
	}

	// Get disk type summary and convert to map keyed by type
	diskTypeSlice, err := p.DiskTypeSummary(ctx, filters)
	if err != nil {
		zap.S().Named("duckdb_parser").Warnf("Failed to get disk type summary: %v", err)
	}
	diskTypes := make(map[string]inventory.DiskTypeSummary, len(diskTypeSlice))
	for _, dt := range diskTypeSlice {
		diskTypes[dt.Type] = dt
	}

	// Get migration issues (returns inventory types directly)
	migrationWarnings, err := p.MigrationIssues(ctx, filters, "Warning")
	if err != nil {
		zap.S().Named("duckdb_parser").Warnf("Failed to get migration warnings: %v", err)
	}

	notMigratableReasons, err := p.MigrationIssues(ctx, filters, "Critical")
	if err != nil {
		zap.S().Named("duckdb_parser").Warnf("Failed to get critical migration issues: %v", err)
	}

	return &inventory.VMsData{
		Total:                       total,
		TotalMigratable:             migratableCount,
		TotalMigratableWithWarnings: migratableWithWarningsCount,
		PowerStates:                 powerStates,
		OSInfo:                      osInfo,
		CPUCores:                    breakdowns.CpuCores,
		RamGB:                       breakdowns.RamGB,
		DiskCount:                   breakdowns.DiskCount,
		DiskGB:                      breakdowns.DiskGB,
		NicCount:                    breakdowns.NicCount,
		DistributionByCPUTier:       cpuTiers,
		DistributionByMemoryTier:    memoryTiers,
		DistributionByNICCount:      nicTiers,
		DiskSizeTiers:               diskSizeTiers,
		DiskTypes:                   diskTypes,
		MigrationWarnings:           migrationWarnings,
		NotMigratableReasons:        notMigratableReasons,
	}, nil
}

// buildInfraData constructs the Infra section of InventoryData.
func (p *Parser) buildInfraData(ctx context.Context, filters Filters) (*inventory.InfraData, error) {
	// Get hosts
	hostModels, err := p.Hosts(ctx, filters, Options{})
	if err != nil {
		zap.S().Named("duckdb_parser").Warnf("Failed to get hosts: %v", err)
	}
	hosts := make([]inventory.Host, 0, len(hostModels))
	for _, h := range hostModels {
		hosts = append(hosts, inventory.Host{
			ID:         h.Id,
			Vendor:     h.Vendor,
			Model:      h.Model,
			CpuCores:   h.CpuCores,
			CpuSockets: h.CpuSockets,
			MemoryMB:   h.MemoryMB,
		})
	}

	// Get host power states
	hostPowerStates, err := p.HostPowerStateCounts(ctx, filters)
	if err != nil {
		zap.S().Named("duckdb_parser").Warnf("Failed to get host power states: %v", err)
		hostPowerStates = make(map[string]int)
	}

	// Get datastores
	datastoreModels, err := p.Datastores(ctx, filters, Options{})
	if err != nil {
		zap.S().Named("duckdb_parser").Warnf("Failed to get datastores: %v", err)
	}
	datastores := make([]inventory.Datastore, 0, len(datastoreModels))
	for _, d := range datastoreModels {
		datastores = append(datastores, inventory.Datastore{
			DiskId:          d.DiskId,
			FreeCapacityGB:  d.FreeCapacityGB,
			TotalCapacityGB: d.TotalCapacityGB,
			Type:            d.Type,
			HostId:          d.HostId,
			Model:           d.Model,
			ProtocolType:    d.ProtocolType,
			Vendor:          d.Vendor,
		})
	}

	// Get networks
	networkModels, err := p.Networks(ctx, filters, Options{})
	if err != nil {
		zap.S().Named("duckdb_parser").Warnf("Failed to get networks: %v", err)
	}

	// Get VM count by network for enrichment
	vmCountByNetwork, err := p.VMCountByNetwork(ctx, filters)
	if err != nil {
		zap.S().Named("duckdb_parser").Warnf("Failed to get VM count by network: %v", err)
		vmCountByNetwork = make(map[string]int)
	}

	networks := make([]inventory.Network, 0, len(networkModels))
	for _, n := range networkModels {
		vmsCount := 0
		if count, ok := vmCountByNetwork[n.Name]; ok {
			vmsCount = count
		}
		networks = append(networks, inventory.Network{
			Name:     n.Name,
			Type:     n.Type,
			Dvswitch: n.Dvswitch,
			VlanId:   n.VlanId,
			VmsCount: vmsCount,
		})
	}

	// Get datacenter count and clusters per datacenter
	// For per-cluster inventories, use fixed values (1 datacenter, 1 cluster)
	var datacenterCount int
	var clustersPerDC []int
	if filters.Cluster != "" {
		// Per-cluster: always 1 datacenter with 1 cluster
		datacenterCount = 1
		clustersPerDC = []int{1}
	} else {
		// vCenter level: get actual counts
		datacenterCount, err = p.DatacenterCount(ctx)
		if err != nil {
			zap.S().Named("duckdb_parser").Warnf("Failed to get datacenter count: %v", err)
		}
		clustersPerDC, err = p.ClustersPerDatacenter(ctx)
		if err != nil {
			zap.S().Named("duckdb_parser").Warnf("Failed to get clusters per datacenter: %v", err)
		}
	}

	// Calculate overcommitment ratios (rounded to 2 decimal places)
	allocatedVCPUs, err := p.AllocatedVCPUs(ctx, filters)
	if err != nil {
		zap.S().Named("duckdb_parser").Warnf("Failed to get allocated vCPUs: %v", err)
	}
	totalHostCPUs, err := p.TotalHostCPUCores(ctx, filters)
	if err != nil {
		zap.S().Named("duckdb_parser").Warnf("Failed to get total host CPUs: %v", err)
	}
	var cpuOvercommit *float64
	if totalHostCPUs > 0 {
		ratio := util.Round(float64(allocatedVCPUs) / float64(totalHostCPUs))
		cpuOvercommit = &ratio
	}

	allocatedMemoryMB, err := p.AllocatedMemoryMB(ctx, filters)
	if err != nil {
		zap.S().Named("duckdb_parser").Warnf("Failed to get allocated memory: %v", err)
	}
	totalHostMemoryMB, err := p.TotalHostMemoryMB(ctx, filters)
	if err != nil {
		zap.S().Named("duckdb_parser").Warnf("Failed to get total host memory: %v", err)
	}
	var memOvercommit *float64
	if totalHostMemoryMB > 0 {
		ratio := util.Round(float64(allocatedMemoryMB) / float64(totalHostMemoryMB))
		memOvercommit = &ratio
	}

	return &inventory.InfraData{
		Hosts:                 hosts,
		HostPowerStates:       hostPowerStates,
		Datastores:            datastores,
		Networks:              networks,
		TotalHosts:            len(hosts),
		TotalDatacenters:      datacenterCount,
		ClustersPerDatacenter: clustersPerDC,
		CPUOverCommitment:     cpuOvercommit,
		MemoryOverCommitment:  memOvercommit,
	}, nil
}

// resolveClusterID determines the cluster ID from vCluster Object ID or generates one.
func resolveClusterID(clusterName string, objectIDs, datacenters map[string]string, vcenterID string) string {
	// Use Object ID from vCluster sheet if available
	if objectID, exists := objectIDs[clusterName]; exists {
		zap.S().Named("duckdb_parser").Debugf("Using vCluster Object ID for '%s' -> '%s'", clusterName, objectID)
		return objectID
	}

	// Fallback: generate anonymous cluster ID
	datacenter := datacenters[clusterName]
	clusterID := generateClusterID(clusterName, datacenter, vcenterID)
	zap.S().Named("duckdb_parser").Warnf("Cluster '%s' not found in vCluster sheet, generated ID: %s", clusterName, clusterID)
	return clusterID
}

// generateClusterID creates a consistent anonymized cluster ID.
// Format: cluster-{first16hexchars} matching the old implementation.
func generateClusterID(clusterName, datacenterName, vcenterUUID string) string {
	// Combine all identifying info for uniqueness
	// Include vcenterUUID to avoid collisions across vCenters
	combined := fmt.Sprintf("%s:%s:%s", vcenterUUID, datacenterName, clusterName)
	hash := sha256.Sum256([]byte(combined))

	// Return as cluster-{first16hexchars}
	return fmt.Sprintf("cluster-%x", hash[:8])
}
