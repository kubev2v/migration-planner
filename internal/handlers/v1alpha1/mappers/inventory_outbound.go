package mappers

import (
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	svcmodel "github.com/kubev2v/migration-planner/internal/model"
)

// This file holds the field-by-field converters from the service-layer domain
// inventory tree to the API model (api/v1alpha1). Each domain struct has an
// explicit conversion function. Primitive fields share the same underlying
// types as the API model and are copied directly; named types are converted.

// InventoryToAPI converts the domain Inventory to the API Inventory.
func InventoryToAPI(inv svcmodel.Inventory) api.Inventory {
	result := api.Inventory{
		CreatedAt:      inv.CreatedAt,
		VcenterId:      inv.VcenterId,
		VcenterVersion: inv.VcenterVersion,
	}
	if inv.Clusters != nil {
		result.Clusters = make(map[string]api.InventoryData, len(inv.Clusters))
		for id, cluster := range inv.Clusters {
			result.Clusters[id] = inventoryDataToAPI(cluster)
		}
	}
	if inv.Vcenter != nil {
		vcenter := inventoryDataToAPI(*inv.Vcenter)
		result.Vcenter = &vcenter
	}
	return result
}

func inventoryDataToAPI(d svcmodel.InventoryData) api.InventoryData {
	result := api.InventoryData{
		Infra: infraToAPI(d.Infra),
		Vms:   vmsToAPI(d.Vms),
	}
	if d.ClusterFeatures != nil {
		features := clusterFeaturesToAPI(*d.ClusterFeatures)
		result.ClusterFeatures = &features
	}
	if d.ClusterUtilization != nil {
		utilization := clusterUtilizationToAPI(*d.ClusterUtilization)
		result.ClusterUtilization = &utilization
	}
	if d.Vcenter != nil {
		vcenter := vcenterToAPI(*d.Vcenter)
		result.Vcenter = &vcenter
	}
	return result
}

func clusterFeaturesToAPI(c svcmodel.ClusterFeatures) api.ClusterFeatures {
	result := api.ClusterFeatures{
		DrsEnabled: c.DrsEnabled,
		HaEnabled:  c.HaEnabled,
	}
	if c.DrsMode != nil {
		mode := api.ClusterFeaturesDrsMode(*c.DrsMode)
		result.DrsMode = &mode
	}
	return result
}

func clusterUtilizationToAPI(c svcmodel.ClusterUtilization) api.ClusterUtilization {
	return api.ClusterUtilization{
		Confidence: c.Confidence,
		CpuAvg:     c.CpuAvg,
		CpuMax:     c.CpuMax,
		CpuP95:     c.CpuP95,
		MemAvg:     c.MemAvg,
		MemMax:     c.MemMax,
		MemP95:     c.MemP95,
	}
}

func infraToAPI(i svcmodel.Infra) api.Infra {
	result := api.Infra{
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
		result.Datastores = make([]api.Datastore, len(i.Datastores))
		for index, datastore := range i.Datastores {
			result.Datastores[index] = datastoreToAPI(datastore)
		}
	}
	if i.Hosts != nil {
		var hosts []api.Host
		if *i.Hosts != nil {
			hosts = make([]api.Host, len(*i.Hosts))
			for index, host := range *i.Hosts {
				hosts[index] = hostToAPI(host)
			}
		}
		result.Hosts = &hosts
	}
	if i.Networks != nil {
		result.Networks = make([]api.Network, len(i.Networks))
		for index, network := range i.Networks {
			result.Networks[index] = networkToAPI(network)
		}
	}
	return result
}

func vcenterToAPI(v svcmodel.VCenter) api.VCenter {
	return api.VCenter{Id: v.Id}
}

func vmsToAPI(v svcmodel.VMs) api.VMs {
	result := api.VMs{
		CpuCores:                    vmResourceBreakdownToAPI(v.CpuCores),
		DiskCount:                   vmResourceBreakdownToAPI(v.DiskCount),
		DiskGB:                      vmResourceBreakdownToAPI(v.DiskGB),
		DistributionByCpuTier:       v.DistributionByCpuTier,
		DistributionByMemoryTier:    v.DistributionByMemoryTier,
		DistributionByNicCount:      v.DistributionByNicCount,
		PowerStates:                 v.PowerStates,
		RamGB:                       vmResourceBreakdownToAPI(v.RamGB),
		Total:                       v.Total,
		TotalMigratable:             v.TotalMigratable,
		TotalMigratableWithWarnings: v.TotalMigratableWithWarnings,
		TotalWithFaultTolerance:     v.TotalWithFaultTolerance,
		TotalWithRDM:                v.TotalWithRDM,
		TotalWithSharedDisks:        v.TotalWithSharedDisks,
	}
	if v.ComplexityDistribution != nil {
		var distribution map[string]api.DiskSizeTierSummary
		if *v.ComplexityDistribution != nil {
			distribution = make(map[string]api.DiskSizeTierSummary, len(*v.ComplexityDistribution))
			for tier, summary := range *v.ComplexityDistribution {
				distribution[tier] = diskSizeTierSummaryToAPI(summary)
			}
		}
		result.ComplexityDistribution = &distribution
	}
	if v.DiskComplexityTier != nil {
		var tiers map[string]api.DiskSizeTierSummary
		if *v.DiskComplexityTier != nil {
			tiers = make(map[string]api.DiskSizeTierSummary, len(*v.DiskComplexityTier))
			for tier, summary := range *v.DiskComplexityTier {
				tiers[tier] = diskSizeTierSummaryToAPI(summary)
			}
		}
		result.DiskComplexityTier = &tiers
	}
	if v.DiskSizeTier != nil {
		var tiers map[string]api.DiskSizeTierSummary
		if *v.DiskSizeTier != nil {
			tiers = make(map[string]api.DiskSizeTierSummary, len(*v.DiskSizeTier))
			for tier, summary := range *v.DiskSizeTier {
				tiers[tier] = diskSizeTierSummaryToAPI(summary)
			}
		}
		result.DiskSizeTier = &tiers
	}
	if v.DiskTypes != nil {
		var types map[string]api.DiskTypeSummary
		if *v.DiskTypes != nil {
			types = make(map[string]api.DiskTypeSummary, len(*v.DiskTypes))
			for diskType, summary := range *v.DiskTypes {
				types[diskType] = diskTypeSummaryToAPI(summary)
			}
		}
		result.DiskTypes = &types
	}
	if v.IssuesBreakdown != nil {
		issues := issuesBreakdownToAPI(*v.IssuesBreakdown)
		result.IssuesBreakdown = &issues
	}
	if v.MigrationWarnings != nil {
		result.MigrationWarnings = make([]api.MigrationIssue, len(v.MigrationWarnings))
		for index, warning := range v.MigrationWarnings {
			result.MigrationWarnings[index] = migrationIssueToAPI(warning)
		}
	}
	if v.NicCount != nil {
		count := vmResourceBreakdownToAPI(*v.NicCount)
		result.NicCount = &count
	}
	if v.NotMigratableReasons != nil {
		result.NotMigratableReasons = make([]api.MigrationIssue, len(v.NotMigratableReasons))
		for index, reason := range v.NotMigratableReasons {
			result.NotMigratableReasons[index] = migrationIssueToAPI(reason)
		}
	}
	if v.OsInfo != nil {
		var info map[string]api.OsInfo
		if *v.OsInfo != nil {
			info = make(map[string]api.OsInfo, len(*v.OsInfo))
			for name, os := range *v.OsInfo {
				info[name] = osInfoToAPI(os)
			}
		}
		result.OsInfo = &info
	}
	return result
}

func datastoreToAPI(d svcmodel.Datastore) api.Datastore {
	result := api.Datastore{
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
		configuration := storageIoConfigurationToAPI(*d.StorageIoConfiguration)
		result.StorageIoConfiguration = &configuration
	}
	if d.StorageProtocol != nil {
		protocol := api.DatastoreStorageProtocol(*d.StorageProtocol)
		result.StorageProtocol = &protocol
	}
	return result
}

func storageIoConfigurationToAPI(s svcmodel.StorageIoConfiguration) api.StorageIoConfiguration {
	result := api.StorageIoConfiguration{
		CongestionThreshold:     s.CongestionThreshold,
		Enabled:                 s.Enabled,
		PercentOfPeakThroughput: s.PercentOfPeakThroughput,
	}
	if s.CongestionThresholdMode != nil {
		mode := api.StorageIoConfigurationCongestionThresholdMode(*s.CongestionThresholdMode)
		result.CongestionThresholdMode = &mode
	}
	return result
}

func hostToAPI(h svcmodel.Host) api.Host {
	return api.Host{
		CpuCores:                h.CpuCores,
		CpuSockets:              h.CpuSockets,
		Id:                      h.Id,
		MemoryMB:                h.MemoryMB,
		Model:                   h.Model,
		StorageVmotionSupported: h.StorageVmotionSupported,
		Vendor:                  h.Vendor,
		VmotionSupported:        h.VmotionSupported,
	}
}

func networkToAPI(n svcmodel.Network) api.Network {
	return api.Network{
		Dvswitch: n.Dvswitch,
		Name:     n.Name,
		Type:     api.NetworkType(n.Type),
		VlanId:   n.VlanId,
		VmsCount: n.VmsCount,
	}
}

func vmResourceBreakdownToAPI(b svcmodel.VMResourceBreakdown) api.VMResourceBreakdown {
	result := api.VMResourceBreakdown{
		Total:                          b.Total,
		TotalForMigratable:             b.TotalForMigratable,
		TotalForMigratableWithWarnings: b.TotalForMigratableWithWarnings,
		TotalForNotMigratable:          b.TotalForNotMigratable,
	}
	if b.Histogram != nil {
		histogram := histogramToAPI(*b.Histogram)
		result.Histogram = &histogram
	}
	return result
}

func histogramToAPI(h svcmodel.Histogram) api.Histogram {
	return api.Histogram{
		Data:     h.Data,
		MinValue: h.MinValue,
		Step:     h.Step,
	}
}

func diskSizeTierSummaryToAPI(s svcmodel.DiskSizeTierSummary) api.DiskSizeTierSummary {
	return api.DiskSizeTierSummary{TotalSizeTB: s.TotalSizeTB, VmCount: s.VmCount}
}

func diskTypeSummaryToAPI(s svcmodel.DiskTypeSummary) api.DiskTypeSummary {
	return api.DiskTypeSummary{TotalSizeTB: s.TotalSizeTB, VmCount: s.VmCount}
}

func issuesBreakdownToAPI(i svcmodel.IssuesBreakdown) api.IssuesBreakdown {
	return api.IssuesBreakdown{
		Advisory:    i.Advisory,
		Critical:    i.Critical,
		Error:       i.Error,
		Information: i.Information,
		Warning:     i.Warning,
	}
}

func migrationIssueToAPI(m svcmodel.MigrationIssue) api.MigrationIssue {
	return api.MigrationIssue{
		Assessment: m.Assessment,
		Count:      m.Count,
		Id:         m.Id,
		Label:      m.Label,
	}
}

func osInfoToAPI(o svcmodel.OsInfo) api.OsInfo {
	result := api.OsInfo{
		Count:                 o.Count,
		Supported:             o.Supported,
		UpgradeRecommendation: o.UpgradeRecommendation,
	}
	if o.SupportTier != nil {
		tier := api.OsInfoSupportTier(*o.SupportTier)
		result.SupportTier = &tier
	}
	return result
}
