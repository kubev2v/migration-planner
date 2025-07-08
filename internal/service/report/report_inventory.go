package report

import (
	"fmt"
	"strings"
	"time"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service/report/types"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

type StandardInventoryProcessor struct{}

func NewStandardInventoryProcessor() *StandardInventoryProcessor {
	return &StandardInventoryProcessor{}
}

func (p *StandardInventoryProcessor) ProcessInventory(source *model.Source) (*types.ReportData, error) {
	if source.Inventory == nil {
		return &types.ReportData{
			Source:     source,
			Inventory:  nil,
			Metrics:    &types.ProcessedMetrics{},
			Timestamps: p.generateTimestamps(),
		}, nil
	}

	inventory := &source.Inventory.Data

	executiveMetrics := p.processExecutiveMetrics(inventory)
	resourceMetrics := p.processResourceMetrics(inventory)
	osDetails := p.processOSDetails(inventory)
	warnings := p.processWarnings(inventory)
	storage := p.processStorageDetails(inventory)
	network := p.processNetworkDetails(inventory)
	hosts := p.processHostDetails(inventory)
	infrastructure := p.processInfrastructureMetrics(inventory)

	processedMetrics := &types.ProcessedMetrics{
		Executive:      executiveMetrics,
		Resources:      resourceMetrics,
		OSDetails:      osDetails,
		Warnings:       warnings,
		Storage:        storage,
		Network:        network,
		Hosts:          hosts,
		Infrastructure: infrastructure,
	}

	return &types.ReportData{
		Source:     source,
		Inventory:  inventory,
		Metrics:    processedMetrics,
		Timestamps: p.generateTimestamps(),
	}, nil
}

func (p *StandardInventoryProcessor) processExecutiveMetrics(inv *v1alpha1.Inventory) types.ExecutiveMetrics {
	poweredOn := 0
	poweredOff := 0
	if states, exists := inv.Vms.PowerStates["poweredOn"]; exists {
		poweredOn = states
	}
	if states, exists := inv.Vms.PowerStates["poweredOff"]; exists {
		poweredOff = states
	}

	return types.ExecutiveMetrics{
		TotalVMs:        inv.Vms.Total,
		PoweredOn:       poweredOn,
		PoweredOff:      poweredOff,
		MigratableVMs:   inv.Vms.TotalMigratable,
		TotalHosts:      inv.Infra.TotalHosts,
		TotalClusters:   inv.Infra.TotalClusters,
		TotalDatastores: len(inv.Infra.Datastores),
		TotalNetworks:   len(inv.Infra.Networks),
	}
}

func (p *StandardInventoryProcessor) processResourceMetrics(inv *v1alpha1.Inventory) types.ResourceMetrics {
	return types.ResourceMetrics{
		CPU: types.ResourceDetail{
			Total:       inv.Vms.CpuCores.Total,
			Average:     float64(inv.Vms.CpuCores.Total) / float64(inv.Vms.Total),
			Recommended: inv.Vms.CpuCores.Total * 120 / 100,
		},
		Memory: types.ResourceDetail{
			Total:       inv.Vms.RamGB.Total,
			Average:     float64(inv.Vms.RamGB.Total) / float64(inv.Vms.Total),
			Recommended: inv.Vms.RamGB.Total * 130 / 100,
		},
		Disk: types.ResourceDetail{
			Total:       inv.Vms.DiskGB.Total,
			Average:     float64(inv.Vms.DiskGB.Total) / float64(inv.Vms.Total),
			Recommended: inv.Vms.DiskGB.Total * 150 / 100,
		},
	}
}

func (p *StandardInventoryProcessor) processOSDetails(inv *v1alpha1.Inventory) []types.OSDetail {
	var osDetails []types.OSDetail

	for osName, count := range inv.Vms.Os {
		priority := "Review Required"
		if strings.Contains(strings.ToLower(osName), "windows") {
			priority = "High"
		} else if strings.Contains(strings.ToLower(osName), "linux") {
			priority = "Medium"
		}

		percentage := float64(count) / float64(inv.Vms.Total) * 100

		osDetails = append(osDetails, types.OSDetail{
			Name:       osName,
			Count:      count,
			Percentage: percentage,
			Priority:   priority,
		})
	}

	return osDetails
}

func (p *StandardInventoryProcessor) processWarnings(inv *v1alpha1.Inventory) []types.WarningDetail {
	var warnings []types.WarningDetail

	for _, warning := range inv.Vms.MigrationWarnings {
		impact := "Low"
		if warning.Count > 50 {
			impact = "Critical"
		} else if warning.Count > 20 {
			impact = "High"
		} else if warning.Count > 5 {
			impact = "Medium"
		}

		percentage := float64(warning.Count) / float64(inv.Vms.Total) * 100
		description := fmt.Sprintf("VMs with %s configuration requiring attention", warning.Label)

		warnings = append(warnings, types.WarningDetail{
			Label:       warning.Label,
			Count:       warning.Count,
			Percentage:  percentage,
			Impact:      impact,
			Description: description,
		})
	}

	return warnings
}

func (p *StandardInventoryProcessor) processStorageDetails(inv *v1alpha1.Inventory) []types.StorageDetail {
	var storage []types.StorageDetail

	for _, ds := range inv.Infra.Datastores {
		utilization := 0.0
		if ds.TotalCapacityGB > 0 {
			utilization = (float64(ds.TotalCapacityGB) - float64(ds.FreeCapacityGB)) / float64(ds.TotalCapacityGB) * 100
		}

		storage = append(storage, types.StorageDetail{
			Vendor:      ds.Vendor,
			Type:        ds.Type,
			Protocol:    ds.ProtocolType,
			TotalGB:     ds.TotalCapacityGB,
			FreeGB:      ds.FreeCapacityGB,
			Utilization: utilization,
			HWAccel:     ds.HardwareAcceleratedMove,
		})
	}

	return storage
}

func (p *StandardInventoryProcessor) processNetworkDetails(inv *v1alpha1.Inventory) []types.NetworkDetail {
	var networks []types.NetworkDetail

	for _, network := range inv.Infra.Networks {
		networks = append(networks, types.NetworkDetail{
			Name:     network.Name,
			Type:     string(network.Type),
			VlanID:   network.VlanId,
			DVSwitch: network.Dvswitch,
		})
	}

	return networks
}

func (p *StandardInventoryProcessor) processHostDetails(inv *v1alpha1.Inventory) []types.HostDetail {
	hostMap := make(map[string]int)

	// Check if Hosts is not nil before ranging over it
	if inv.Infra.Hosts != nil {
		for _, host := range *inv.Infra.Hosts {
			key := fmt.Sprintf("%s-%s", host.Vendor, host.Model)
			hostMap[key]++
		}
	}

	var hosts []types.HostDetail
	for key, count := range hostMap {
		parts := strings.Split(key, "-")
		if len(parts) >= 2 {
			hosts = append(hosts, types.HostDetail{
				Vendor: parts[0],
				Model:  strings.Join(parts[1:], "-"),
				Count:  count,
			})
		}
	}

	return hosts
}

func (p *StandardInventoryProcessor) processInfrastructureMetrics(inv *v1alpha1.Inventory) types.InfrastructureMetrics {
	clustersPerDatacenter := make([]int, 0)
	hostPowerStates := make(map[string]int)

	// Use ClustersPerDatacenter if available
	if inv.Infra.ClustersPerDatacenter != nil {
		clustersPerDatacenter = *inv.Infra.ClustersPerDatacenter
	}

	// Use HostsPerCluster directly as it's not a pointer
	hostsPerCluster := inv.Infra.HostsPerCluster

	for state, count := range inv.Infra.HostPowerStates {
		hostPowerStates[state] = count
	}

	// Handle TotalDatacenters which is a pointer to int
	totalDatacenters := 0
	if inv.Infra.TotalDatacenters != nil {
		totalDatacenters = *inv.Infra.TotalDatacenters
	}

	return types.InfrastructureMetrics{
		TotalDatacenters:      totalDatacenters,
		TotalClusters:         inv.Infra.TotalClusters,
		TotalHosts:            inv.Infra.TotalHosts,
		ClustersPerDatacenter: clustersPerDatacenter,
		HostsPerCluster:       hostsPerCluster,
		HostPowerStates:       hostPowerStates,
	}
}

func (p *StandardInventoryProcessor) generateTimestamps() types.ReportTimestamps {
	now := time.Now()
	return types.ReportTimestamps{
		Generated:     now.Format("2006-01-02"),
		GeneratedTime: now.Format("15:04:05"),
	}
}
