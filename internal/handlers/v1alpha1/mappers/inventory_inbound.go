package mappers

import (
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	svcmodel "github.com/kubev2v/migration-planner/internal/model"
)

// InventoryFromAPI converts an API inventory to the structured service model.
func InventoryFromAPI(inv api.Inventory) svcmodel.Inventory {
	result := svcmodel.Inventory{
		CollectionTime: inv.CollectionTime,
		CreatedAt:      inv.CreatedAt,
		VcenterId:      inv.VcenterId,
		VcenterVersion: inv.VcenterVersion,
	}
	if inv.Clusters != nil {
		result.Clusters = make(map[string]svcmodel.InventoryData, len(inv.Clusters))
		for id, cluster := range inv.Clusters {
			result.Clusters[id] = inventoryDataFromAPI(cluster)
		}
	}
	if inv.Vcenter != nil {
		vcenter := inventoryDataFromAPI(*inv.Vcenter)
		result.Vcenter = &vcenter
	}
	return result
}

func inventoryDataFromAPI(d api.InventoryData) svcmodel.InventoryData {
	result := svcmodel.InventoryData{
		Infra: infraFromAPI(d.Infra),
		Vms:   vmsFromAPI(d.Vms),
	}
	if d.ClusterFeatures != nil {
		features := clusterFeaturesFromAPI(*d.ClusterFeatures)
		result.ClusterFeatures = &features
	}
	if d.ClusterUtilization != nil {
		utilization := clusterUtilizationFromAPI(*d.ClusterUtilization)
		result.ClusterUtilization = &utilization
	}
	if d.Vcenter != nil {
		vcenter := vcenterFromAPI(*d.Vcenter)
		result.Vcenter = &vcenter
	}
	return result
}

func clusterFeaturesFromAPI(c api.ClusterFeatures) svcmodel.ClusterFeatures {
	result := svcmodel.ClusterFeatures{
		DrsEnabled: c.DrsEnabled,
		HaEnabled:  c.HaEnabled,
	}
	if c.DrsMode != nil {
		mode := svcmodel.ClusterFeaturesDrsMode(*c.DrsMode)
		result.DrsMode = &mode
	}
	return result
}

func clusterUtilizationFromAPI(c api.ClusterUtilization) svcmodel.ClusterUtilization {
	return svcmodel.ClusterUtilization{
		Confidence: c.Confidence,
		CpuAvg:     c.CpuAvg,
		CpuMax:     c.CpuMax,
		CpuP95:     c.CpuP95,
		MemAvg:     c.MemAvg,
		MemMax:     c.MemMax,
		MemP95:     c.MemP95,
	}
}

func infraFromAPI(i api.Infra) svcmodel.Infra {
	result := svcmodel.Infra{
		ClustersPerDatacenter:   i.ClustersPerDatacenter,
		CpuOverCommitment:       i.CpuOverCommitment,
		HostPowerStates:         i.HostPowerStates,
		MemoryOverCommitment:    i.MemoryOverCommitment,
		StandaloneHostsDetected: i.StandaloneHostsDetected,
		TotalDatacenters:        i.TotalDatacenters,
		TotalHosts:              i.TotalHosts,
		VmsPerHostAverage:       i.VmsPerHostAverage,
	}
	if i.Datastores != nil {
		result.Datastores = make([]svcmodel.Datastore, len(i.Datastores))
		for index, datastore := range i.Datastores {
			result.Datastores[index] = datastoreFromAPI(datastore)
		}
	}
	if i.Hosts != nil {
		var hosts []svcmodel.Host
		if *i.Hosts != nil {
			hosts = make([]svcmodel.Host, len(*i.Hosts))
			for index, host := range *i.Hosts {
				hosts[index] = hostFromAPI(host)
			}
		}
		result.Hosts = &hosts
	}
	if i.Networks != nil {
		result.Networks = make([]svcmodel.Network, len(i.Networks))
		for index, network := range i.Networks {
			result.Networks[index] = networkFromAPI(network)
		}
	}
	return result
}

func vcenterFromAPI(v api.VCenter) svcmodel.VCenter {
	return svcmodel.VCenter{Id: v.Id}
}

func vmsFromAPI(v api.VMs) svcmodel.VMs {
	result := svcmodel.VMs{
		CpuCores:                    vmResourceBreakdownFromAPI(v.CpuCores),
		DiskCount:                   vmResourceBreakdownFromAPI(v.DiskCount),
		DiskGB:                      vmResourceBreakdownFromAPI(v.DiskGB),
		DistributionByCpuTier:       v.DistributionByCpuTier,
		DistributionByMemoryTier:    v.DistributionByMemoryTier,
		DistributionByNicCount:      v.DistributionByNicCount,
		PowerStates:                 v.PowerStates,
		RamGB:                       vmResourceBreakdownFromAPI(v.RamGB),
		Total:                       v.Total,
		TotalMigratable:             v.TotalMigratable,
		TotalMigratableWithWarnings: v.TotalMigratableWithWarnings,
		TotalWithFaultTolerance:     v.TotalWithFaultTolerance,
		TotalWithRDM:                v.TotalWithRDM,
		TotalWithSharedDisks:        v.TotalWithSharedDisks,
	}
	if v.ComplexityDistribution != nil {
		var distribution map[string]svcmodel.DiskSizeTierSummary
		if *v.ComplexityDistribution != nil {
			distribution = make(map[string]svcmodel.DiskSizeTierSummary, len(*v.ComplexityDistribution))
			for tier, summary := range *v.ComplexityDistribution {
				distribution[tier] = diskSizeTierSummaryFromAPI(summary)
			}
		}
		result.ComplexityDistribution = &distribution
	}
	if v.DiskComplexityTier != nil {
		var tiers map[string]svcmodel.DiskSizeTierSummary
		if *v.DiskComplexityTier != nil {
			tiers = make(map[string]svcmodel.DiskSizeTierSummary, len(*v.DiskComplexityTier))
			for tier, summary := range *v.DiskComplexityTier {
				tiers[tier] = diskSizeTierSummaryFromAPI(summary)
			}
		}
		result.DiskComplexityTier = &tiers
	}
	if v.DiskSizeTier != nil {
		var tiers map[string]svcmodel.DiskSizeTierSummary
		if *v.DiskSizeTier != nil {
			tiers = make(map[string]svcmodel.DiskSizeTierSummary, len(*v.DiskSizeTier))
			for tier, summary := range *v.DiskSizeTier {
				tiers[tier] = diskSizeTierSummaryFromAPI(summary)
			}
		}
		result.DiskSizeTier = &tiers
	}
	if v.DiskTypes != nil {
		var types map[string]svcmodel.DiskTypeSummary
		if *v.DiskTypes != nil {
			types = make(map[string]svcmodel.DiskTypeSummary, len(*v.DiskTypes))
			for diskType, summary := range *v.DiskTypes {
				types[diskType] = diskTypeSummaryFromAPI(summary)
			}
		}
		result.DiskTypes = &types
	}
	if v.IssuesBreakdown != nil {
		issues := issuesBreakdownFromAPI(*v.IssuesBreakdown)
		result.IssuesBreakdown = &issues
	}
	if v.MigrationWarnings != nil {
		result.MigrationWarnings = make([]svcmodel.MigrationIssue, len(v.MigrationWarnings))
		for index, warning := range v.MigrationWarnings {
			result.MigrationWarnings[index] = migrationIssueFromAPI(warning)
		}
	}
	if v.NicCount != nil {
		count := vmResourceBreakdownFromAPI(*v.NicCount)
		result.NicCount = &count
	}
	if v.NotMigratableReasons != nil {
		result.NotMigratableReasons = make([]svcmodel.MigrationIssue, len(v.NotMigratableReasons))
		for index, reason := range v.NotMigratableReasons {
			result.NotMigratableReasons[index] = migrationIssueFromAPI(reason)
		}
	}
	if v.OsInfo != nil {
		var info map[string]svcmodel.OsInfo
		if *v.OsInfo != nil {
			info = make(map[string]svcmodel.OsInfo, len(*v.OsInfo))
			for name, os := range *v.OsInfo {
				info[name] = osInfoFromAPI(os)
			}
		}
		result.OsInfo = &info
	}
	return result
}

func datastoreFromAPI(d api.Datastore) svcmodel.Datastore {
	result := svcmodel.Datastore{
		BackingDeviceIds:        d.BackingDeviceIds,
		FilesystemMajorVersion:  d.FilesystemMajorVersion,
		FilesystemType:          d.FilesystemType,
		DiskId:                  d.DiskId,
		FreeCapacityGB:          d.FreeCapacityGB,
		HardwareAcceleratedMove: d.HardwareAcceleratedMove,
		HostId:                  d.HostId,
		Model:                   d.Model,
		ProtocolType:            d.ProtocolType,
		TotalCapacityGB:         d.TotalCapacityGB,
		Type:                    d.Type,
		Vendor:                  d.Vendor,
		VsanSnapshotSupport:     d.VsanSnapshotSupport,
	}
	if d.StorageIoConfiguration != nil {
		configuration := storageIoConfigurationFromAPI(*d.StorageIoConfiguration)
		result.StorageIoConfiguration = &configuration
	}
	if d.StorageProtocol != nil {
		protocol := svcmodel.DatastoreStorageProtocol(*d.StorageProtocol)
		result.StorageProtocol = &protocol
	}
	return result
}

func storageIoConfigurationFromAPI(s api.StorageIoConfiguration) svcmodel.StorageIoConfiguration {
	result := svcmodel.StorageIoConfiguration{
		CongestionThreshold:     s.CongestionThreshold,
		Enabled:                 s.Enabled,
		PercentOfPeakThroughput: s.PercentOfPeakThroughput,
	}
	if s.CongestionThresholdMode != nil {
		mode := svcmodel.StorageIoConfigurationCongestionThresholdMode(*s.CongestionThresholdMode)
		result.CongestionThresholdMode = &mode
	}
	return result
}

func hostFromAPI(h api.Host) svcmodel.Host {
	result := svcmodel.Host{
		CpuCores:                h.CpuCores,
		CpuSockets:              h.CpuSockets,
		Id:                      h.Id,
		MemoryMB:                h.MemoryMB,
		Model:                   h.Model,
		StorageVmotionSupported: h.StorageVmotionSupported,
		Vendor:                  h.Vendor,
		VmotionSupported:        h.VmotionSupported,
	}
	if h.StorageAdapters != nil {
		adapters := make([]svcmodel.HostStorageAdapter, 0, len(*h.StorageAdapters))
		for _, adapter := range *h.StorageAdapters {
			adapters = append(adapters, svcmodel.HostStorageAdapter(adapter))
		}
		result.StorageAdapters = &adapters
	}
	if h.StorageLuns != nil {
		luns := make([]svcmodel.HostStorageLun, 0, len(*h.StorageLuns))
		for _, lun := range *h.StorageLuns {
			luns = append(luns, svcmodel.HostStorageLun(lun))
		}
		result.StorageLuns = &luns
	}
	return result
}

func networkFromAPI(n api.Network) svcmodel.Network {
	return svcmodel.Network{
		Dvswitch: n.Dvswitch,
		Name:     n.Name,
		Type:     svcmodel.NetworkType(n.Type),
		VlanId:   n.VlanId,
		VmsCount: n.VmsCount,
	}
}

func vmResourceBreakdownFromAPI(b api.VMResourceBreakdown) svcmodel.VMResourceBreakdown {
	result := svcmodel.VMResourceBreakdown{
		Total:                          b.Total,
		TotalForMigratable:             b.TotalForMigratable,
		TotalForMigratableWithWarnings: b.TotalForMigratableWithWarnings,
		TotalForNotMigratable:          b.TotalForNotMigratable,
	}
	if b.Histogram != nil {
		histogram := histogramFromAPI(*b.Histogram)
		result.Histogram = &histogram
	}
	return result
}

func histogramFromAPI(h api.Histogram) svcmodel.Histogram {
	return svcmodel.Histogram{
		Data:     h.Data,
		MinValue: h.MinValue,
		Step:     h.Step,
	}
}

func diskSizeTierSummaryFromAPI(s api.DiskSizeTierSummary) svcmodel.DiskSizeTierSummary {
	return svcmodel.DiskSizeTierSummary{TotalSizeTB: s.TotalSizeTB, VmCount: s.VmCount}
}

func diskTypeSummaryFromAPI(s api.DiskTypeSummary) svcmodel.DiskTypeSummary {
	return svcmodel.DiskTypeSummary{TotalSizeTB: s.TotalSizeTB, VmCount: s.VmCount}
}

func issuesBreakdownFromAPI(i api.IssuesBreakdown) svcmodel.IssuesBreakdown {
	return svcmodel.IssuesBreakdown{
		Advisory:    i.Advisory,
		Critical:    i.Critical,
		Error:       i.Error,
		Information: i.Information,
		Warning:     i.Warning,
	}
}

func migrationIssueFromAPI(m api.MigrationIssue) svcmodel.MigrationIssue {
	return svcmodel.MigrationIssue{
		Assessment: m.Assessment,
		Count:      m.Count,
		Id:         m.Id,
		Label:      m.Label,
	}
}

func osInfoFromAPI(o api.OsInfo) svcmodel.OsInfo {
	result := svcmodel.OsInfo{
		Count:                 o.Count,
		Supported:             o.Supported,
		UpgradeRecommendation: o.UpgradeRecommendation,
	}
	if o.SupportTier != nil {
		tier := svcmodel.OsInfoSupportTier(*o.SupportTier)
		result.SupportTier = &tier
	}
	return result
}
