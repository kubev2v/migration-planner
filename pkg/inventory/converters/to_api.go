package converters

import (
	"strings"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/pkg/inventory"
)

// ToAPI converts domain Inventory to api.Inventory.
func ToAPI(inv *inventory.Inventory) *api.Inventory {
	clusters := make(map[string]api.InventoryData)
	for clusterID, data := range inv.Clusters {
		clusters[clusterID] = toAPIInventoryData(&data)
	}

	result := &api.Inventory{
		VcenterId: inv.VCenterID,
		Clusters:  clusters,
	}
	if inv.VCenter != nil {
		vcenter := toAPIInventoryData(inv.VCenter)
		result.Vcenter = &vcenter
	}
	return result
}

func toAPIInventoryData(d *inventory.InventoryData) api.InventoryData {
	return api.InventoryData{
		Vms:   toAPIVMs(&d.VMs),
		Infra: toAPIInfra(&d.Infra),
	}
}

func toAPIVMs(v *inventory.VMsData) api.VMs {
	osInfo := make(map[string]api.OsInfo)
	for name, os := range v.OSInfo {
		info := api.OsInfo{
			Count:     os.Count,
			Supported: os.IsSupported,
		}
		if os.UpgradeRecommendation != "" {
			rec := os.UpgradeRecommendation
			info.UpgradeRecommendation = &rec
		}
		osInfo[name] = info
	}

	diskSizeTiers := make(map[string]api.DiskSizeTierSummary)
	for tier, summary := range v.DiskSizeTiers {
		diskSizeTiers[tier] = api.DiskSizeTierSummary{
			VmCount:     summary.VMCount,
			TotalSizeTB: summary.TotalSizeTB,
		}
	}

	diskTypes := make(map[string]api.DiskTypeSummary)
	for diskType, summary := range v.DiskTypes {
		diskTypes[diskType] = api.DiskTypeSummary{
			VmCount:     summary.VMCount,
			TotalSizeTB: summary.TotalSizeTB,
		}
	}

	migrationWarnings := make([]api.MigrationIssue, 0, len(v.MigrationWarnings))
	for _, w := range v.MigrationWarnings {
		id := w.ID
		migrationWarnings = append(migrationWarnings, api.MigrationIssue{
			Id:         &id,
			Label:      w.Label,
			Assessment: w.Assessment,
			Count:      w.Count,
		})
	}

	notMigratableReasons := make([]api.MigrationIssue, 0, len(v.NotMigratableReasons))
	for _, c := range v.NotMigratableReasons {
		id := c.ID
		notMigratableReasons = append(notMigratableReasons, api.MigrationIssue{
			Id:         &id,
			Label:      c.Label,
			Assessment: c.Assessment,
			Count:      c.Count,
		})
	}

	nicCount := api.VMResourceBreakdown{
		Total:                          v.NicCount.Total,
		TotalForMigratable:             v.NicCount.TotalForMigratable,
		TotalForMigratableWithWarnings: v.NicCount.TotalForMigratableWithWarnings,
		TotalForNotMigratable:          v.NicCount.TotalForNotMigratable,
	}

	migratableWithWarnings := v.TotalMigratableWithWarnings

	return api.VMs{
		Total:                       v.Total,
		TotalMigratable:             v.TotalMigratable,
		TotalMigratableWithWarnings: &migratableWithWarnings,
		PowerStates:                 v.PowerStates,
		OsInfo:                      &osInfo,
		CpuCores: api.VMResourceBreakdown{
			Total:                          v.CPUCores.Total,
			TotalForMigratable:             v.CPUCores.TotalForMigratable,
			TotalForMigratableWithWarnings: v.CPUCores.TotalForMigratableWithWarnings,
			TotalForNotMigratable:          v.CPUCores.TotalForNotMigratable,
		},
		RamGB: api.VMResourceBreakdown{
			Total:                          v.RamGB.Total,
			TotalForMigratable:             v.RamGB.TotalForMigratable,
			TotalForMigratableWithWarnings: v.RamGB.TotalForMigratableWithWarnings,
			TotalForNotMigratable:          v.RamGB.TotalForNotMigratable,
		},
		DiskCount: api.VMResourceBreakdown{
			Total:                          v.DiskCount.Total,
			TotalForMigratable:             v.DiskCount.TotalForMigratable,
			TotalForMigratableWithWarnings: v.DiskCount.TotalForMigratableWithWarnings,
			TotalForNotMigratable:          v.DiskCount.TotalForNotMigratable,
		},
		DiskGB: api.VMResourceBreakdown{
			Total:                          v.DiskGB.Total,
			TotalForMigratable:             v.DiskGB.TotalForMigratable,
			TotalForMigratableWithWarnings: v.DiskGB.TotalForMigratableWithWarnings,
			TotalForNotMigratable:          v.DiskGB.TotalForNotMigratable,
		},
		NicCount:                 &nicCount,
		DistributionByCpuTier:    &v.DistributionByCPUTier,
		DistributionByMemoryTier: &v.DistributionByMemoryTier,
		DistributionByNicCount:   &v.DistributionByNICCount,
		DiskSizeTier:             &diskSizeTiers,
		DiskTypes:                &diskTypes,
		MigrationWarnings:        migrationWarnings,
		NotMigratableReasons:     notMigratableReasons,
	}
}

func toAPIInfra(i *inventory.InfraData) api.Infra {
	hosts := make([]api.Host, 0, len(i.Hosts))
	for _, h := range i.Hosts {
		host := api.Host{
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
		hosts = append(hosts, host)
	}

	datastores := make([]api.Datastore, 0, len(i.Datastores))
	for _, d := range i.Datastores {
		ds := api.Datastore{
			DiskId:                  d.DiskId,
			FreeCapacityGB:          int(d.FreeCapacityGB),
			TotalCapacityGB:         int(d.TotalCapacityGB),
			Type:                    d.Type,
			HardwareAcceleratedMove: false, // Always false to match old behavior
			Model:                   d.Model,
			ProtocolType:            d.ProtocolType,
			Vendor:                  d.Vendor,
		}
		if d.HostId != "" {
			hostId := d.HostId
			ds.HostId = &hostId
		}
		// Anonymize NFS datastores
		anonymizeNFSDatastore(&ds)
		datastores = append(datastores, ds)
	}

	networks := make([]api.Network, 0, len(i.Networks))
	for _, n := range i.Networks {
		net := api.Network{
			Name: n.Name,
			Type: api.NetworkType(n.Type),
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

	infra := api.Infra{
		Hosts:                &hosts,
		HostPowerStates:      i.HostPowerStates,
		Datastores:           datastores,
		Networks:             networks,
		TotalHosts:           i.TotalHosts,
		CpuOverCommitment:    i.CPUOverCommitment,
		MemoryOverCommitment: i.MemoryOverCommitment,
	}

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
func anonymizeNFSDatastore(ds *api.Datastore) {
	if strings.EqualFold(ds.Type, "NFS") {
		ds.DiskId = "N/A"
		ds.ProtocolType = "N/A"
	}
}
