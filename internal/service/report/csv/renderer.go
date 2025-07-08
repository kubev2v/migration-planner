package csv

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"time"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service/report/types"
)

type Renderer struct{}

func NewRenderer() *Renderer {
	return &Renderer{}
}

func (r *Renderer) SupportedFormat() types.ReportFormat {
	return types.ReportFormatCSV
}

func (r *Renderer) Render(data *types.ReportData) (string, error) {
	if data.Inventory == nil {
		return r.generateEmptyReport(data)
	}

	var csvRows [][]string

	csvRows = append(csvRows, []string{"VMWARE INFRASTRUCTURE ASSESSMENT REPORT"})
	csvRows = append(csvRows, []string{fmt.Sprintf("Generated: %s at %s",
		data.Timestamps.Generated, data.Timestamps.GeneratedTime)})
	csvRows = append(csvRows, []string{""})

	csvRows = r.addExecutiveSummary(csvRows, data.Metrics.Executive)
	csvRows = r.addResourceAllocationSummary(csvRows, data.Metrics.Resources)
	csvRows = r.addOperatingSystemDistribution(csvRows, data.Metrics.OSDetails)

	if data.Options.IncludeWarnings {
		csvRows = r.addMigrationWarnings(csvRows, data.Metrics.Warnings)
	}

	csvRows = r.addStorageAnalysis(csvRows, data.Metrics.Storage)
	csvRows = r.addNetworkTopology(csvRows, data.Metrics.Network)
	csvRows = r.addHostInformation(csvRows, data.Metrics.Hosts)
	csvRows = r.addInfrastructureSummary(csvRows, data.Metrics.Infrastructure)
	csvRows = r.addVMResourceDistribution(csvRows, data.Inventory)
	csvRows = r.addResourceTotalsSummary(csvRows, data.Inventory)

	return r.convertRowsToCSV(csvRows)
}

func (r *Renderer) generateEmptyReport(data *types.ReportData) (string, error) {
	csvRows := [][]string{
		{"VMWARE INFRASTRUCTURE ASSESSMENT REPORT"},
		{fmt.Sprintf("Generated: %s at %s", data.Timestamps.Generated, data.Timestamps.GeneratedTime)},
		{""},
		{"NOTICE"},
		{""},
		{"No inventory data available for this source."},
		{"Please upload RVTools data or run discovery agent to populate inventory."},
		{""},
		{"Source Information", "Value"},
		{"Source Name", data.Source.Name},
		{"Source ID", data.Source.ID.String()},
		{"Created At", data.Source.CreatedAt.Format(time.RFC3339)},
		{"On Premises", fmt.Sprintf("%v", data.Source.OnPremises)},
	}

	return r.convertRowsToCSV(csvRows)
}

func (r *Renderer) addExecutiveSummary(csvRows [][]string, metrics types.ExecutiveMetrics) [][]string {
	csvRows = append(csvRows, []string{"EXECUTIVE SUMMARY"})
	csvRows = append(csvRows, []string{""})
	csvRows = append(csvRows, []string{"Metric", "Value", "Details"})

	csvRows = append(csvRows, []string{
		"Total Virtual Machines",
		fmt.Sprintf("%d", metrics.TotalVMs),
		fmt.Sprintf("%d powered on, %d powered off", metrics.PoweredOn, metrics.PoweredOff)})
	csvRows = append(csvRows, []string{
		"ESXi Hosts",
		fmt.Sprintf("%d", metrics.TotalHosts),
		"Physical servers running ESXi hypervisor"})
	csvRows = append(csvRows, []string{
		"vSphere Clusters",
		fmt.Sprintf("%d", metrics.TotalClusters),
		"High availability and resource pooling clusters"})
	csvRows = append(csvRows, []string{
		"Datastores",
		fmt.Sprintf("%d", metrics.TotalDatastores),
		"Shared storage repositories for VM files"})
	csvRows = append(csvRows, []string{
		"Virtual Networks",
		fmt.Sprintf("%d", metrics.TotalNetworks),
		"Network configurations and VLAN settings"})
	csvRows = append(csvRows, []string{
		"Migration Candidates",
		fmt.Sprintf("%d", metrics.MigratableVMs),
		"VMs ready for migration with warnings"})
	csvRows = append(csvRows, []string{""})

	return csvRows
}

func (r *Renderer) addResourceAllocationSummary(csvRows [][]string, metrics types.ResourceMetrics) [][]string {
	csvRows = append(csvRows, []string{"RESOURCE ALLOCATION SUMMARY"})
	csvRows = append(csvRows, []string{""})
	csvRows = append(csvRows, []string{"Resource Type", "Total Allocated", "Average per VM", "Recommended Cluster Size"})

	csvRows = append(csvRows, []string{
		"CPU Cores (vCPUs)",
		fmt.Sprintf("%d", metrics.CPU.Total),
		fmt.Sprintf("%.1f", metrics.CPU.Average),
		fmt.Sprintf("%d", metrics.CPU.Recommended)})
	csvRows = append(csvRows, []string{
		"Memory (GB)",
		fmt.Sprintf("%d", metrics.Memory.Total),
		fmt.Sprintf("%.1f", metrics.Memory.Average),
		fmt.Sprintf("%d", metrics.Memory.Recommended)})
	csvRows = append(csvRows, []string{
		"Storage (GB)",
		fmt.Sprintf("%d", metrics.Disk.Total),
		fmt.Sprintf("%.1f", metrics.Disk.Average),
		fmt.Sprintf("%d", metrics.Disk.Recommended)})
	csvRows = append(csvRows, []string{""})

	return csvRows
}

func (r *Renderer) addOperatingSystemDistribution(csvRows [][]string, osDetails []types.OSDetail) [][]string {
	csvRows = append(csvRows, []string{"OPERATING SYSTEM DISTRIBUTION"})
	csvRows = append(csvRows, []string{""})
	csvRows = append(csvRows, []string{"Operating System", "VM Count", "Percentage", "Migration Priority"})

	for _, os := range osDetails {
		csvRows = append(csvRows, []string{
			os.Name,
			fmt.Sprintf("%d", os.Count),
			fmt.Sprintf("%.1f%%", os.Percentage),
			os.Priority})
	}
	csvRows = append(csvRows, []string{""})

	return csvRows
}

func (r *Renderer) addMigrationWarnings(csvRows [][]string, warnings []types.WarningDetail) [][]string {
	if len(warnings) == 0 {
		return csvRows
	}

	csvRows = append(csvRows, []string{"MIGRATION WARNINGS"})
	csvRows = append(csvRows, []string{""})
	csvRows = append(csvRows, []string{"Warning Category", "Affected VMs", "Impact Level", "Percentage", "Description"})

	for _, warning := range warnings {
		csvRows = append(csvRows, []string{
			warning.Label,
			fmt.Sprintf("%d", warning.Count),
			warning.Impact,
			fmt.Sprintf("%.1f%%", warning.Percentage),
			warning.Description})
	}
	csvRows = append(csvRows, []string{""})

	return csvRows
}

func (r *Renderer) addStorageAnalysis(csvRows [][]string, storage []types.StorageDetail) [][]string {
	if len(storage) == 0 {
		return csvRows
	}

	csvRows = append(csvRows, []string{"STORAGE ANALYSIS"})
	csvRows = append(csvRows, []string{""})
	csvRows = append(csvRows, []string{"Type", "Vendor", "Storage Support", "Protocol Type", "Model", "Total Capacity (GB)", "Free Capacity (GB)", "Utilization %", "Hardware Acceleration"})

	for _, ds := range storage {
		hwAccel := "No"
		if ds.HWAccel {
			hwAccel = "Yes"
		}
		csvRows = append(csvRows, []string{
			ds.Type,
			ds.Vendor,
			"", // Storage Support - not in struct
			ds.Protocol,
			"", // Model - not in struct currently
			fmt.Sprintf("%d", ds.TotalGB),
			fmt.Sprintf("%d", ds.FreeGB),
			fmt.Sprintf("%.1f%%", ds.Utilization),
			hwAccel,
		})
	}
	csvRows = append(csvRows, []string{""})

	return csvRows
}

func (r *Renderer) addNetworkTopology(csvRows [][]string, networks []types.NetworkDetail) [][]string {
	if len(networks) == 0 {
		return csvRows
	}

	csvRows = append(csvRows, []string{"NETWORK TOPOLOGY"})
	csvRows = append(csvRows, []string{""})
	csvRows = append(csvRows, []string{"Name", "Type", "VLAN ID", "DVSwitch"})

	for _, network := range networks {
		vlanId := ""
		if network.VlanID != nil {
			vlanId = *network.VlanID
		}
		dvswitch := ""
		if network.DVSwitch != nil {
			dvswitch = *network.DVSwitch
		}
		csvRows = append(csvRows, []string{
			network.Name,
			network.Type,
			vlanId,
			dvswitch,
		})
	}
	csvRows = append(csvRows, []string{""})

	return csvRows
}

func (r *Renderer) addHostInformation(csvRows [][]string, hosts []types.HostDetail) [][]string {
	if len(hosts) == 0 {
		return csvRows
	}

	csvRows = append(csvRows, []string{"HOST INFORMATION"})
	csvRows = append(csvRows, []string{""})
	csvRows = append(csvRows, []string{"Vendor", "Model", "Count"})

	for _, host := range hosts {
		csvRows = append(csvRows, []string{host.Vendor, host.Model, fmt.Sprintf("%d", host.Count)})
	}
	csvRows = append(csvRows, []string{""})

	return csvRows
}

func (r *Renderer) addInfrastructureSummary(csvRows [][]string, infra types.InfrastructureMetrics) [][]string {
	csvRows = append(csvRows, []string{"INFRASTRUCTURE SUMMARY"})
	csvRows = append(csvRows, []string{""})
	csvRows = append(csvRows, []string{"Component", "Count", "Details"})
	csvRows = append(csvRows, []string{"Datacenters", fmt.Sprintf("%d", infra.TotalDatacenters), "Physical locations"})
	csvRows = append(csvRows, []string{"Clusters", fmt.Sprintf("%d", infra.TotalClusters), "vSphere clusters"})
	csvRows = append(csvRows, []string{"Total Hosts", fmt.Sprintf("%d", infra.TotalHosts), "ESXi hosts across all clusters"})

	if len(infra.ClustersPerDatacenter) > 0 {
		csvRows = append(csvRows, []string{""})
		csvRows = append(csvRows, []string{"CLUSTERS PER DATACENTER"})
		csvRows = append(csvRows, []string{""})
		csvRows = append(csvRows, []string{"Datacenter", "Cluster Count"})
		for i, clusterCount := range infra.ClustersPerDatacenter {
			csvRows = append(csvRows, []string{fmt.Sprintf("Datacenter %d", i+1), fmt.Sprintf("%d", clusterCount)})
		}
		csvRows = append(csvRows, []string{""})
	}

	return csvRows
}

func (r *Renderer) addVMResourceDistribution(csvRows [][]string, inv *v1alpha1.Inventory) [][]string {
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
		csvRows = r.addHistogramSection(csvRows, config)
	}

	return csvRows
}

func (r *Renderer) addResourceTotalsSummary(csvRows [][]string, inv *v1alpha1.Inventory) [][]string {
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

	return csvRows
}

func (r *Renderer) addHistogramSection(csvRows [][]string, config histogramConfig) [][]string {
	csvRows = append(csvRows, []string{config.title})
	csvRows = append(csvRows, []string{"Range", "VM Count"})

	if len(config.histogram.Data) > 0 {
		minVal := config.histogram.MinValue
		step := config.histogram.Step
		for i, count := range config.histogram.Data {
			if count > 0 {
				rangeStart := minVal + (i * step)
				rangeEnd := rangeStart + step - 1
				csvRows = append(csvRows, []string{
					fmt.Sprintf("%d-%d %s", rangeStart, rangeEnd, config.unit),
					fmt.Sprintf("%d", count),
				})
			}
		}
	} else {
		csvRows = append(csvRows, []string{config.noDataMsg, "0"})
	}
	csvRows = append(csvRows, []string{""})

	return csvRows
}

func (r *Renderer) convertRowsToCSV(csvRows [][]string) (string, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	for _, row := range csvRows {
		if err := writer.Write(row); err != nil {
			return "", fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", fmt.Errorf("failed to flush CSV writer: %w", err)
	}

	return buf.String(), nil
}

// histogramConfig is a helper struct for CSV histogram processing
type histogramConfig struct {
	histogram v1alpha1.Histogram
	title     string
	unit      string
	noDataMsg string
}
