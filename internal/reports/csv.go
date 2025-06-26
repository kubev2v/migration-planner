package reports

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kubev2v/migration-planner/internal/store/model"
)

func GenerateCSVContent(source *model.Source, reportType string, includeWarnings bool) (string, error) {
	if source.Inventory == nil {
		return GenerateEmptyCSVReport(getReportMetadata(source))
	}

	inventoryData, err := source.Inventory.Value()
	if err != nil {
		return GenerateEmptyCSVReport(getReportMetadata(source))
	}

	inv, err := ParseInventory(inventoryData)
	if err != nil {
		return GenerateEmptyCSVReport(getReportMetadata(source))
	}

	var csvRows [][]string
	
	// Title
	csvRows = append(csvRows, []string{"VMWARE INFRASTRUCTURE ASSESSMENT REPORT"})
	csvRows = append(csvRows, []string{fmt.Sprintf("Generated: %s at %s", 
		time.Now().Format("01/02/2006"), 
		time.Now().Format("3:04:05 PM"))})
	csvRows = append(csvRows, []string{""})
	
	// Executive Summary
	csvRows = append(csvRows, []string{"EXECUTIVE SUMMARY"})
	csvRows = append(csvRows, []string{""})
	csvRows = append(csvRows, []string{"Metric", "Value", "Details"})
	
	poweredOn := 0
	poweredOff := 0
	if count, exists := inv.Vms.PowerStates["poweredOn"]; exists {
		poweredOn = count
	}
	if count, exists := inv.Vms.PowerStates["poweredOff"]; exists {
		poweredOff = count
	}
	
	migratableVMs := inv.Vms.TotalMigratable
	if inv.Vms.TotalMigratableWithWarnings != nil {
		migratableVMs = *inv.Vms.TotalMigratableWithWarnings
	}
	
	csvRows = append(csvRows, []string{
		"Total Virtual Machines", 
		fmt.Sprintf("%d", inv.Vms.Total), 
		fmt.Sprintf("%d powered on, %d powered off", poweredOn, poweredOff)})
	csvRows = append(csvRows, []string{
		"ESXi Hosts", 
		fmt.Sprintf("%d", inv.Infra.TotalHosts), 
		"Physical servers running ESXi hypervisor"})
	csvRows = append(csvRows, []string{
		"vSphere Clusters", 
		fmt.Sprintf("%d", inv.Infra.TotalClusters), 
		"High availability and resource pooling clusters"})
	csvRows = append(csvRows, []string{
		"Datastores", 
		fmt.Sprintf("%d", len(inv.Infra.Datastores)), 
		"Shared storage repositories for VM files"})
	csvRows = append(csvRows, []string{
		"Virtual Networks", 
		fmt.Sprintf("%d", len(inv.Infra.Networks)), 
		"Network configurations and VLAN settings"})
	csvRows = append(csvRows, []string{
		"Migration Candidates", 
		fmt.Sprintf("%d", migratableVMs), 
		"VMs ready for migration with warnings"})
	csvRows = append(csvRows, []string{""})
	
	// Resource Allocation Summary
	csvRows = append(csvRows, []string{"RESOURCE ALLOCATION SUMMARY"})
	csvRows = append(csvRows, []string{""})
	csvRows = append(csvRows, []string{"Resource Type", "Total Allocated", "Average per VM", "Recommended Cluster Size"})
	
	if inv.Vms.Total > 0 {
		csvRows = append(csvRows, []string{
			"CPU Cores (vCPUs)", 
			fmt.Sprintf("%d", inv.Vms.CpuCores.Total), 
			fmt.Sprintf("%.1f", float64(inv.Vms.CpuCores.Total)/float64(inv.Vms.Total)), 
			fmt.Sprintf("%d", int(float64(inv.Vms.CpuCores.Total)*1.2))})
		csvRows = append(csvRows, []string{
			"Memory (GB)", 
			fmt.Sprintf("%d", inv.Vms.RamGB.Total), 
			fmt.Sprintf("%.1f", float64(inv.Vms.RamGB.Total)/float64(inv.Vms.Total)), 
			fmt.Sprintf("%d", int(float64(inv.Vms.RamGB.Total)*1.25))})
		csvRows = append(csvRows, []string{
			"Storage (GB)", 
			fmt.Sprintf("%d", inv.Vms.DiskGB.Total), 
			fmt.Sprintf("%.1f", float64(inv.Vms.DiskGB.Total)/float64(inv.Vms.Total)), 
			fmt.Sprintf("%d", int(float64(inv.Vms.DiskGB.Total)*1.15))})
	}
	csvRows = append(csvRows, []string{""})
	
	// Operating System Distribution
	csvRows = append(csvRows, []string{"OPERATING SYSTEM DISTRIBUTION"})
	csvRows = append(csvRows, []string{""})
	csvRows = append(csvRows, []string{"Operating System", "VM Count", "Percentage", "Migration Priority"})
	
	// Sort OS by count (descending)
	type osEntry struct {
		name  string
		count int
	}
	var osEntries []osEntry
	for osName, count := range inv.Vms.Os {
		osEntries = append(osEntries, osEntry{name: osName, count: count})
	}
	sort.Slice(osEntries, func(i, j int) bool {
		return osEntries[i].count > osEntries[j].count
	})
	
	for _, os := range osEntries {
		percentage := fmt.Sprintf("%.1f%%", (float64(os.count)/float64(inv.Vms.Total))*100)
		priority := getOSPriority(os.name)
		csvRows = append(csvRows, []string{os.name, fmt.Sprintf("%d", os.count), percentage, priority})
	}
	
	// Migration Warnings
	if includeWarnings && len(inv.Vms.MigrationWarnings) > 0 {
		csvRows = append(csvRows, []string{"MIGRATION WARNINGS"})
		csvRows = append(csvRows, []string{""})
		csvRows = append(csvRows, []string{"Warning Category", "Affected VMs", "Impact Level", "Percentage", "Description"})
		
		for _, warning := range inv.Vms.MigrationWarnings {
			impact := getImpactLevel(warning.Count)
			percentage := fmt.Sprintf("%.1f%%", (float64(warning.Count)/float64(inv.Vms.Total))*100)
			csvRows = append(csvRows, []string{
				warning.Label, 
				fmt.Sprintf("%d", warning.Count), 
				impact, 
				percentage, 
				warning.Assessment})
		}
		csvRows = append(csvRows, []string{""})
	}
	
	// Storage Analysis
	if len(inv.Infra.Datastores) > 0 {
		csvRows = append(csvRows, []string{"STORAGE ANALYSIS"})
		csvRows = append(csvRows, []string{""})
		csvRows = append(csvRows, []string{"Type", "Vendor", "Storage Support", "Protocol Type", "Model", "Total Capacity (GB)", "Free Capacity (GB)", "Utilization %", "Hardware Acceleration"})
		
		for _, ds := range inv.Infra.Datastores {
			utilization := "0.0"
			if ds.TotalCapacityGB > 0 {
				util := ((float64(ds.TotalCapacityGB) - float64(ds.FreeCapacityGB)) / float64(ds.TotalCapacityGB)) * 100
				utilization = fmt.Sprintf("%.1f", util)
			}
			hwAccel := "No"
			if ds.HardwareAcceleratedMove {
				hwAccel = "Yes"
			}
			csvRows = append(csvRows, []string{
				ds.Type,
				ds.Vendor,
				"", // Storage Support - not in our struct
				ds.ProtocolType,
				ds.Model,
				fmt.Sprintf("%d", ds.TotalCapacityGB),
				fmt.Sprintf("%d", ds.FreeCapacityGB),
				utilization + "%",
				hwAccel,
			})
		}
		csvRows = append(csvRows, []string{""})
	}
	
	// Network Topology
	if len(inv.Infra.Networks) > 0 {
		csvRows = append(csvRows, []string{"NETWORK TOPOLOGY"})
		csvRows = append(csvRows, []string{""})
		csvRows = append(csvRows, []string{"Name", "Type", "VLAN ID", "DVSwitch"})
		
		for _, network := range inv.Infra.Networks {
			vlanId := ""
			if network.VlanId != nil {
				vlanId = *network.VlanId
			}
			dvswitch := ""
			if network.Dvswitch != nil {
				dvswitch = *network.Dvswitch
			}
			csvRows = append(csvRows, []string{
				network.Name,
				string(network.Type),
				vlanId,
				dvswitch,
			})
		}
		csvRows = append(csvRows, []string{""})
	}
	
	// Host Information
	if len(inv.Infra.Hosts) > 0 {
		csvRows = append(csvRows, []string{"HOST INFORMATION"})
		csvRows = append(csvRows, []string{""})
		csvRows = append(csvRows, []string{"Vendor", "Model", "Count"})
		
		// Group hosts by vendor/model
		hostCounts := make(map[string]int)
		for _, host := range inv.Infra.Hosts {
			key := fmt.Sprintf("%s|%s", host.Vendor, host.Model)
			hostCounts[key]++
		}
		
		for hostKey, count := range hostCounts {
			parts := strings.Split(hostKey, "|")
			if len(parts) == 2 {
				csvRows = append(csvRows, []string{parts[0], parts[1], fmt.Sprintf("%d", count)})
			}
		}
		csvRows = append(csvRows, []string{""})
	}
	
	// Infrastructure Summary
	csvRows = append(csvRows, []string{"INFRASTRUCTURE SUMMARY"})
	csvRows = append(csvRows, []string{""})
	csvRows = append(csvRows, []string{"Component", "Count", "Details"})
	csvRows = append(csvRows, []string{"Datacenters", fmt.Sprintf("%d", inv.Infra.TotalDatacenters), "Physical locations"})
	csvRows = append(csvRows, []string{"Clusters", fmt.Sprintf("%d", inv.Infra.TotalClusters), "vSphere clusters"})
	csvRows = append(csvRows, []string{"Total Hosts", fmt.Sprintf("%d", inv.Infra.TotalHosts), "ESXi hosts across all clusters"})
	
	// Clusters per datacenter breakdown
	if len(inv.Infra.ClustersPerDatacenter) > 0 {
		csvRows = append(csvRows, []string{""})
		csvRows = append(csvRows, []string{"CLUSTERS PER DATACENTER"})
		csvRows = append(csvRows, []string{""})
		csvRows = append(csvRows, []string{"Datacenter", "Cluster Count"})
		for i, clusterCount := range inv.Infra.ClustersPerDatacenter {
			csvRows = append(csvRows, []string{fmt.Sprintf("Datacenter %d", i+1), fmt.Sprintf("%d", clusterCount)})
		}
		csvRows = append(csvRows, []string{""})
	}
	
	// Hosts per cluster breakdown
	if len(inv.Infra.HostsPerCluster) > 0 {
		csvRows = append(csvRows, []string{"HOSTS PER CLUSTER"})
		csvRows = append(csvRows, []string{""})
		csvRows = append(csvRows, []string{"Cluster", "Host Count"})
		for i, hostCount := range inv.Infra.HostsPerCluster {
			csvRows = append(csvRows, []string{fmt.Sprintf("Cluster %d", i+1), fmt.Sprintf("%d", hostCount)})
		}
		csvRows = append(csvRows, []string{""})
	}
	
	// Host Power States
	if len(inv.Infra.HostPowerStates) > 0 {
		csvRows = append(csvRows, []string{"HOST POWER STATES"})
		csvRows = append(csvRows, []string{""})
		csvRows = append(csvRows, []string{"Power State", "Host Count", "Percentage"})
		for state, count := range inv.Infra.HostPowerStates {
			percentage := fmt.Sprintf("%.1f%%", (float64(count)/float64(inv.Infra.TotalHosts))*100)
			csvRows = append(csvRows, []string{state, fmt.Sprintf("%d", count), percentage})
		}
		csvRows = append(csvRows, []string{""})
	}
	
	// VM Resource Distribution Histograms
	csvRows = append(csvRows, []string{"VM RESOURCE DISTRIBUTION"})
	csvRows = append(csvRows, []string{""})

	histograms := []histogramConfig{
		{
			histogram: inv.Vms.CpuCores.Histogram,
			title:     "CPU CORES DISTRIBUTION",
			unit:      "cores",
			noDataMsg: "No CPU distribution data available",
		},
		{
			histogram: inv.Vms.RamGB.Histogram,
			title:     "MEMORY (RAM) DISTRIBUTION", 
			unit:      "GB",
			noDataMsg: "No RAM distribution data available",
		},
		{
			histogram: inv.Vms.DiskGB.Histogram,
			title:     "DISK STORAGE DISTRIBUTION",
			unit:      "GB",
			noDataMsg: "No disk distribution data available",
		},
	}

	for _, config := range histograms {
		addHistogramSection(&csvRows, config)
	}
	
	// VM Resource Totals Summary
	csvRows = append(csvRows, []string{"RESOURCE TOTALS BY MIGRATION STATUS"})
	csvRows = append(csvRows, []string{""})
	csvRows = append(csvRows, []string{"Resource Type", "Total", "Migratable", "Migratable with Warnings", "Not Migratable"})
	csvRows = append(csvRows, []string{
		"CPU Cores",
		fmt.Sprintf("%d", inv.Vms.CpuCores.Total),
		fmt.Sprintf("%d", inv.Vms.CpuCores.TotalForMigratable),
		fmt.Sprintf("%d", inv.Vms.CpuCores.TotalForMigratableWithWarnings),
		fmt.Sprintf("%d", inv.Vms.CpuCores.TotalForNotMigratable),
	})
	csvRows = append(csvRows, []string{
		"Memory (GB)",
		fmt.Sprintf("%d", inv.Vms.RamGB.Total),
		fmt.Sprintf("%d", inv.Vms.RamGB.TotalForMigratable),
		fmt.Sprintf("%d", inv.Vms.RamGB.TotalForMigratableWithWarnings),
		fmt.Sprintf("%d", inv.Vms.RamGB.TotalForNotMigratable),
	})
	csvRows = append(csvRows, []string{
		"Disk Storage (GB)",
		fmt.Sprintf("%d", inv.Vms.DiskGB.Total),
		fmt.Sprintf("%d", inv.Vms.DiskGB.TotalForMigratable),
		fmt.Sprintf("%d", inv.Vms.DiskGB.TotalForMigratableWithWarnings),
		fmt.Sprintf("%d", inv.Vms.DiskGB.TotalForNotMigratable),
	})
	
	csvContent, err := convertRowsToCSV(csvRows)
	if err != nil {
		return "", fmt.Errorf("failed to convert data to CSV format: %w", err)
	}

return csvContent, nil
}