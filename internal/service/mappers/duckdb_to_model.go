package mappers

import (
	"math"
	"strings"

	"github.com/kubev2v/migration-planner/internal/service/model"

	"github.com/kubev2v/migration-planner/pkg/inventory"
)

// DuckdbToModel converts duckdb Inventory to domain Inventory.
func DuckdbToModel(inv *inventory.Inventory) *model.Inventory {
	clusters := make(map[string]model.InventoryData)
	for clusterID, data := range inv.Clusters {
		clusters[clusterID] = duckdbToDomainInventoryData(&data)
	}

	result := &model.Inventory{
		VcenterId:      inv.VCenterID,
		Clusters:       clusters,
		VcenterVersion: &inv.VCenterVersion,
		CreatedAt:      inv.CreatedAt,
	}
	if inv.VCenter != nil {
		vcenter := duckdbToDomainInventoryData(inv.VCenter)
		result.Vcenter = &vcenter
	}

	return result
}

func duckdbToDomainInventoryData(d *inventory.InventoryData) model.InventoryData {
	result := model.InventoryData{
		Vms:   duckdbToDomainVMs(&d.VMs),
		Infra: duckdbToDomainInfra(&d.Infra),
	}
	result.Infra.VmsPerHostAverage = vmsPerHostAverage(d.VMs.Total, d.Infra.TotalHosts)

	if d.ClusterFeatures != nil {
		clusterFeatures := model.ClusterFeatures{
			DrsEnabled: d.ClusterFeatures.DrsEnabled,
			HaEnabled:  d.ClusterFeatures.HaEnabled,
		}

		// Map DrsMode string to enum type
		if d.ClusterFeatures.DrsMode != nil {
			normalized := strings.ToLower(strings.TrimSpace(*d.ClusterFeatures.DrsMode))
			switch normalized {
			case "fullyautomated", "fully automated":
				mode := model.ClusterFeaturesDrsModeFullyAutomated
				clusterFeatures.DrsMode = &mode
			case "partiallyautomated", "partially automated":
				mode := model.ClusterFeaturesDrsModePartiallyAutomated
				clusterFeatures.DrsMode = &mode
			case "manual":
				mode := model.ClusterFeaturesDrsModeManual
				clusterFeatures.DrsMode = &mode
			default:
				mode := model.ClusterFeaturesDrsModeNone
				clusterFeatures.DrsMode = &mode
			}
		}

		result.ClusterFeatures = &clusterFeatures
	}

	if d.ClusterUtilization != nil {
		util := model.ClusterUtilization{
			CpuAvg:     d.ClusterUtilization.CpuAvg,
			CpuP95:     d.ClusterUtilization.CpuP95,
			CpuMax:     d.ClusterUtilization.CpuMax,
			MemAvg:     d.ClusterUtilization.MemAvg,
			MemP95:     d.ClusterUtilization.MemP95,
			MemMax:     d.ClusterUtilization.MemMax,
			Confidence: d.ClusterUtilization.Confidence,
		}
		result.ClusterUtilization = &util
	}

	return result
}

func duckdbToDomainVMs(v *inventory.VMsData) model.VMs {
	osInfo := make(map[string]model.OsInfo)
	for name, os := range v.OSInfo {
		tier := model.OsInfoSupportTier(os.SupportTier)
		info := model.OsInfo{
			Count:       os.Count,
			Supported:   os.IsSupported,
			SupportTier: &tier,
		}
		if os.UpgradeRecommendation != "" {
			rec := os.UpgradeRecommendation
			info.UpgradeRecommendation = &rec
		}
		osInfo[name] = info
	}

	diskSizeTiers := make(map[string]model.DiskSizeTierSummary)
	for tier, summary := range v.DiskSizeTiers {
		diskSizeTiers[tier] = model.DiskSizeTierSummary{
			VmCount:     summary.VMCount,
			TotalSizeTB: summary.TotalSizeTB,
		}
	}

	var diskComplexityTierPtr *map[string]model.DiskSizeTierSummary
	if len(v.DiskComplexityTiers) > 0 {
		m := make(map[string]model.DiskSizeTierSummary, len(v.DiskComplexityTiers))
		for tier, summary := range v.DiskComplexityTiers {
			m[tier] = model.DiskSizeTierSummary{
				VmCount:     summary.VMCount,
				TotalSizeTB: summary.TotalSizeTB,
			}
		}
		diskComplexityTierPtr = &m
	}

	diskTypes := make(map[string]model.DiskTypeSummary)
	for diskType, summary := range v.DiskTypes {
		diskTypes[diskType] = model.DiskTypeSummary{
			VmCount:     summary.VMCount,
			TotalSizeTB: summary.TotalSizeTB,
		}
	}

	complexityDist := make(map[string]model.DiskSizeTierSummary, len(v.ComplexityDistribution))
	for level, summary := range v.ComplexityDistribution {
		complexityDist[level] = model.DiskSizeTierSummary{
			VmCount:     summary.VMCount,
			TotalSizeTB: summary.TotalSizeTB,
		}
	}

	migrationWarnings := make([]model.MigrationIssue, 0, len(v.MigrationWarnings))
	for _, w := range v.MigrationWarnings {
		id := w.ID
		migrationWarnings = append(migrationWarnings, model.MigrationIssue{
			Id:         &id,
			Label:      w.Label,
			Assessment: w.Assessment,
			Count:      w.Count,
		})
	}

	notMigratableReasons := make([]model.MigrationIssue, 0, len(v.NotMigratableReasons))
	for _, c := range v.NotMigratableReasons {
		id := c.ID
		notMigratableReasons = append(notMigratableReasons, model.MigrationIssue{
			Id:         &id,
			Label:      c.Label,
			Assessment: c.Assessment,
			Count:      c.Count,
		})
	}

	nicCount := model.VMResourceBreakdown{
		Total:                          v.NicCount.Total,
		TotalForMigratable:             v.NicCount.TotalForMigratable,
		TotalForMigratableWithWarnings: v.NicCount.TotalForMigratableWithWarnings,
		TotalForNotMigratable:          v.NicCount.TotalForNotMigratable,
	}

	migratableWithWarnings := v.TotalMigratableWithWarnings
	totalWithSharedDisks := v.TotalWithSharedDisks
	totalWithRDM := v.TotalWithRDM
	totalWithFaultTolerance := v.TotalWithFaultTolerance

	return model.VMs{
		Total:                       v.Total,
		TotalMigratable:             v.TotalMigratable,
		TotalMigratableWithWarnings: &migratableWithWarnings,
		TotalWithSharedDisks:        &totalWithSharedDisks,
		TotalWithRDM:                &totalWithRDM,
		TotalWithFaultTolerance:     &totalWithFaultTolerance,
		PowerStates:                 v.PowerStates,
		OsInfo:                      &osInfo,
		CpuCores: model.VMResourceBreakdown{
			Total:                          v.CPUCores.Total,
			TotalForMigratable:             v.CPUCores.TotalForMigratable,
			TotalForMigratableWithWarnings: v.CPUCores.TotalForMigratableWithWarnings,
			TotalForNotMigratable:          v.CPUCores.TotalForNotMigratable,
		},
		RamGB: model.VMResourceBreakdown{
			Total:                          v.RamGB.Total,
			TotalForMigratable:             v.RamGB.TotalForMigratable,
			TotalForMigratableWithWarnings: v.RamGB.TotalForMigratableWithWarnings,
			TotalForNotMigratable:          v.RamGB.TotalForNotMigratable,
		},
		DiskCount: model.VMResourceBreakdown{
			Total:                          v.DiskCount.Total,
			TotalForMigratable:             v.DiskCount.TotalForMigratable,
			TotalForMigratableWithWarnings: v.DiskCount.TotalForMigratableWithWarnings,
			TotalForNotMigratable:          v.DiskCount.TotalForNotMigratable,
		},
		DiskGB: model.VMResourceBreakdown{
			Total:                          v.DiskGB.Total,
			TotalForMigratable:             v.DiskGB.TotalForMigratable,
			TotalForMigratableWithWarnings: v.DiskGB.TotalForMigratableWithWarnings,
			TotalForNotMigratable:          v.DiskGB.TotalForNotMigratable,
		},
		NicCount:                 &nicCount,
		DistributionByCpuTier:    &v.DistributionByCPUTier,
		DistributionByMemoryTier: &v.DistributionByMemoryTier,
		DistributionByNicCount:   &v.DistributionByNICCount,
		ComplexityDistribution:   &complexityDist,
		DiskSizeTier:             &diskSizeTiers,
		DiskComplexityTier:       diskComplexityTierPtr,
		DiskTypes:                &diskTypes,
		MigrationWarnings:        migrationWarnings,
		NotMigratableReasons:     notMigratableReasons,
		IssuesBreakdown: &model.IssuesBreakdown{
			Critical:    v.IssuesBreakdown.Critical,
			Warning:     v.IssuesBreakdown.Warning,
			Information: v.IssuesBreakdown.Information,
			Advisory:    v.IssuesBreakdown.Advisory,
			Error:       v.IssuesBreakdown.Error,
		},
	}
}

func duckdbToDomainInfra(i *inventory.InfraData) model.Infra {
	hosts := make([]model.Host, 0, len(i.Hosts))
	for _, h := range i.Hosts {
		host := model.Host{
			Vendor: h.Vendor,
			Model:  h.Model,
		}
		if h.ID != "" {
			id := h.ID
			host.Id = &id
		}
		if h.CpuCores > 0 {
			cores := h.CpuCores
			host.CpuCores = &cores
		}
		if h.CpuSockets > 0 {
			cpus := h.CpuSockets
			host.CpuSockets = &cpus
		}
		if h.MemoryMB > 0 {
			mem := int64(h.MemoryMB)
			host.MemoryMB = &mem
		}
		host.VmotionSupported = h.VMotionSupported
		host.StorageVmotionSupported = h.StorageVMotionSupported
		hosts = append(hosts, host)
	}

	datastores := make([]model.Datastore, 0, len(i.Datastores))
	for _, d := range i.Datastores {
		vsanSnapshotSupport := d.VsanSnapshotSupport
		ds := model.Datastore{
			DiskId:                  d.DiskId,
			FreeCapacityGB:          int(d.FreeCapacityGB),
			TotalCapacityGB:         int(d.TotalCapacityGB),
			Type:                    d.Type,
			VsanSnapshotSupport:     &vsanSnapshotSupport,
			HardwareAcceleratedMove: false, // Always false to match old behavior
			Model:                   d.Model,
			ProtocolType:            d.ProtocolType,
			Vendor:                  d.Vendor,
		}
		if d.StorageProtocol != "" {
			storageProtocol := model.DatastoreStorageProtocol(d.StorageProtocol)
			ds.StorageProtocol = &storageProtocol
		}
		if d.HostId != "" {
			hostId := d.HostId
			ds.HostId = &hostId
		}
		if d.StorageIoConfiguration != nil {
			enabled := d.StorageIoConfiguration.Enabled
			congestion := d.StorageIoConfiguration.CongestionThreshold
			mode := model.StorageIoConfigurationCongestionThresholdMode(d.StorageIoConfiguration.CongestionThresholdMode)
			percent := d.StorageIoConfiguration.PercentOfPeakThroughput
			ds.StorageIoConfiguration = &model.StorageIoConfiguration{
				Enabled:                 &enabled,
				CongestionThreshold:     &congestion,
				CongestionThresholdMode: &mode,
				PercentOfPeakThroughput: &percent,
			}
		}
		anonymizeNFSDatastore(&ds)
		datastores = append(datastores, ds)
	}

	networks := make([]model.Network, 0, len(i.Networks))
	for _, n := range i.Networks {
		net := model.Network{
			Name: n.Name,
			Type: model.NetworkType(n.Type),
		}
		if n.Dvswitch != "" {
			dvs := n.Dvswitch
			net.Dvswitch = &dvs
		}
		if n.VlanId != "" {
			vlan := n.VlanId
			net.VlanId = &vlan
		}
		if n.VmsCount > 0 {
			count := n.VmsCount
			net.VmsCount = &count
		}
		networks = append(networks, net)
	}

	infra := model.Infra{
		Hosts:                &hosts,
		HostPowerStates:      i.HostPowerStates,
		Datastores:           datastores,
		Networks:             networks,
		TotalHosts:           i.TotalHosts,
		CpuOverCommitment:    i.CPUOverCommitment,
		MemoryOverCommitment: i.MemoryOverCommitment,
	}

	standaloneHostsDetected := i.StandaloneHostsDetected
	infra.StandaloneHostsDetected = &standaloneHostsDetected

	if i.TotalDatacenters > 0 {
		dc := i.TotalDatacenters
		infra.TotalDatacenters = &dc
	}
	if len(i.ClustersPerDatacenter) > 0 {
		infra.ClustersPerDatacenter = &i.ClustersPerDatacenter
	}

	return infra
}

// anonymizeNFSDatastore sets diskId and protocolType to "N/A" for NFS datastores.
// NFS paths may contain sensitive server information that should not be exposed.
func anonymizeNFSDatastore(ds *model.Datastore) {
	if strings.EqualFold(ds.Type, "NFS") {
		ds.DiskId = "N/A"
		ds.ProtocolType = "N/A"
	}
}

// vmsPerHostAverage returns the average VM density per host, rounded to 2
// decimals. Returns nil when there are no hosts (division undefined).
func vmsPerHostAverage(totalVMs, totalHosts int) *float64 {
	if totalHosts <= 0 {
		return nil
	}
	avg := math.Round(float64(totalVMs*100)/float64(totalHosts)) / 100
	return &avg
}
