package service

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

type ReportService struct {
}

func NewReportService() *ReportService {
	return &ReportService{}
}

func (r *ReportService) GenerateReport(source *model.Source, options ReportOptions) (string, error) {
	switch options.Format {
	case ReportFormatCSV:
		return r.generateCSVReport(source, options)
	case ReportFormatHTML:
		return r.generateHTMLReport(source, options)
	default:
		return "", fmt.Errorf("unsupported report format: %s", options.Format)
	}
}

func (r *ReportService) generateCSVReport(source *model.Source, options ReportOptions) (string, error) {
	if source.Inventory == nil {
		return r.generateEmptyCSVReport(source)
	}

	inventoryData, err := source.Inventory.Value()
	if err != nil {
		return r.generateEmptyCSVReport(source)
	}

	inv, err := r.parseInventory(inventoryData)
	if err != nil {
		return r.generateEmptyCSVReport(source)
	}

	var csvRows [][]string

	csvRows = append(csvRows, []string{"VMWARE INFRASTRUCTURE ASSESSMENT REPORT"})
	csvRows = append(csvRows, []string{fmt.Sprintf("Generated: %s at %s",
		time.Now().Format("01/02/2006"),
		time.Now().Format("3:04:05 PM"))})
	csvRows = append(csvRows, []string{""})

	csvRows = r.addExecutiveSummary(csvRows, inv)

	csvRows = r.addResourceAllocationSummary(csvRows, inv)

	csvRows = r.addOperatingSystemDistribution(csvRows, inv)

	if options.IncludeWarnings {
		csvRows = r.addMigrationWarnings(csvRows, inv)
	}

	csvRows = r.addStorageAnalysis(csvRows, inv)

	csvRows = r.addNetworkTopology(csvRows, inv)

	csvRows = r.addHostInformation(csvRows, inv)

	csvRows = r.addInfrastructureSummary(csvRows, inv)

	csvRows = r.addVMResourceDistribution(csvRows, inv)

	csvRows = r.addResourceTotalsSummary(csvRows, inv)

	return r.convertRowsToCSV(csvRows)
}

func (r *ReportService) generateHTMLReport(source *model.Source, options ReportOptions) (string, error) {
	if source == nil {
		return "", errors.New("source cannot be nil")
	}

	if source.Inventory == nil {
		return r.generateEmptyHTMLReport(source)
	}

	inventoryData, err := source.Inventory.Value()
	if err != nil {
		return "", fmt.Errorf("failed to extract inventory data: %w", err)
	}

	inv, err := r.parseInventory(inventoryData)
	if err != nil {
		return "", fmt.Errorf("failed to parse inventory data: %w", err)
	}

	var osEntries []osEntry
	for osName, count := range inv.Vms.Os {
		osEntries = append(osEntries, osEntry{name: osName, count: count})
	}
	sort.Slice(osEntries, func(i, j int) bool {
		return osEntries[i].count > osEntries[j].count
	})

	data := ReportTemplateData{
		CSS:           r.getInteractiveCSS(),
		GeneratedDate: time.Now().Format("1/2/2006"),
		GeneratedTime: time.Now().Format("3:04:05 PM"),

		TotalVMs:        inv.Vms.Total,
		TotalHosts:      inv.Infra.TotalHosts,
		TotalDatastores: len(inv.Infra.Datastores),
		TotalNetworks:   len(inv.Infra.Networks),

		OSTable:              r.generateOSTable(osEntries, inv.Vms.Total),
		DiskSizeTable:        r.generateDiskSizeTable(inv, inv.Vms.Total),
		StorageTable:         r.generateStorageTable(inv.Infra.Datastores),
		WarningsTableSection: r.generateWarningsTableSection(inv.Vms.MigrationWarnings, inv.Vms.Total, options.IncludeWarnings),
		WarningsChartSection: r.generateWarningsChartSection(inv.Vms.MigrationWarnings, options.IncludeWarnings),

		CPUTotal:           inv.Vms.CpuCores.Total,
		CPUAverage:         fmt.Sprintf("%.1f", float64(inv.Vms.CpuCores.Total)/float64(inv.Vms.Total)),
		CPURecommended:     int(float64(inv.Vms.CpuCores.Total) * 1.2),
		MemoryTotal:        inv.Vms.RamGB.Total,
		MemoryAverage:      fmt.Sprintf("%.1f", float64(inv.Vms.RamGB.Total)/float64(inv.Vms.Total)),
		MemoryRecommended:  int(float64(inv.Vms.RamGB.Total) * 1.25),
		StorageTotal:       inv.Vms.DiskGB.Total,
		StorageAverage:     fmt.Sprintf("%.1f", float64(inv.Vms.DiskGB.Total)/float64(inv.Vms.Total)),
		StorageRecommended: int(float64(inv.Vms.DiskGB.Total) * 1.15),

		JavaScript: r.generateInteractiveChartJS(inv, osEntries, options.IncludeWarnings),
	}

	return r.executeTemplate(htmlReportTemplate, data)
}

// Helper Methods for CSV Generation
func (r *ReportService) addExecutiveSummary(csvRows [][]string, inv v1alpha1.Inventory) [][]string {
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

	return csvRows
}

func (r *ReportService) addResourceAllocationSummary(csvRows [][]string, inv v1alpha1.Inventory) [][]string {
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

	return csvRows
}

func (r *ReportService) addOperatingSystemDistribution(csvRows [][]string, inv v1alpha1.Inventory) [][]string {
	csvRows = append(csvRows, []string{"OPERATING SYSTEM DISTRIBUTION"})
	csvRows = append(csvRows, []string{""})
	csvRows = append(csvRows, []string{"Operating System", "VM Count", "Percentage", "Migration Priority"})

	var osEntries []osEntry
	for osName, count := range inv.Vms.Os {
		osEntries = append(osEntries, osEntry{name: osName, count: count})
	}
	sort.Slice(osEntries, func(i, j int) bool {
		return osEntries[i].count > osEntries[j].count
	})

	for _, os := range osEntries {
		percentage := fmt.Sprintf("%.1f%%", (float64(os.count)/float64(inv.Vms.Total))*100)
		priority := r.getOSPriority(os.name)
		csvRows = append(csvRows, []string{os.name, fmt.Sprintf("%d", os.count), percentage, priority})
	}
	csvRows = append(csvRows, []string{""})

	return csvRows
}

func (r *ReportService) addMigrationWarnings(csvRows [][]string, inv v1alpha1.Inventory) [][]string {
	if len(inv.Vms.MigrationWarnings) == 0 {
		return csvRows
	}

	csvRows = append(csvRows, []string{"MIGRATION WARNINGS"})
	csvRows = append(csvRows, []string{""})
	csvRows = append(csvRows, []string{"Warning Category", "Affected VMs", "Impact Level", "Percentage", "Description"})

	for _, warning := range inv.Vms.MigrationWarnings {
		impact := r.getImpactLevel(warning.Count)
		percentage := fmt.Sprintf("%.1f%%", (float64(warning.Count)/float64(inv.Vms.Total))*100)
		csvRows = append(csvRows, []string{
			warning.Label,
			fmt.Sprintf("%d", warning.Count),
			impact,
			percentage,
			warning.Assessment})
	}
	csvRows = append(csvRows, []string{""})

	return csvRows
}

func (r *ReportService) addStorageAnalysis(csvRows [][]string, inv v1alpha1.Inventory) [][]string {
	if len(inv.Infra.Datastores) == 0 {
		return csvRows
	}

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
			"", // Storage Support - not in struct
			ds.ProtocolType,
			ds.Model,
			fmt.Sprintf("%d", ds.TotalCapacityGB),
			fmt.Sprintf("%d", ds.FreeCapacityGB),
			utilization + "%",
			hwAccel,
		})
	}
	csvRows = append(csvRows, []string{""})

	return csvRows
}

func (r *ReportService) addNetworkTopology(csvRows [][]string, inv v1alpha1.Inventory) [][]string {
	if len(inv.Infra.Networks) == 0 {
		return csvRows
	}

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

	return csvRows
}

func (r *ReportService) addHostInformation(csvRows [][]string, inv v1alpha1.Inventory) [][]string {
	if len(inv.Infra.Hosts) == 0 {
		return csvRows
	}

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

	return csvRows
}

func (r *ReportService) addInfrastructureSummary(csvRows [][]string, inv v1alpha1.Inventory) [][]string {
	csvRows = append(csvRows, []string{"INFRASTRUCTURE SUMMARY"})
	csvRows = append(csvRows, []string{""})
	csvRows = append(csvRows, []string{"Component", "Count", "Details"})
	csvRows = append(csvRows, []string{"Datacenters", fmt.Sprintf("%d", inv.Infra.TotalDatacenters), "Physical locations"})
	csvRows = append(csvRows, []string{"Clusters", fmt.Sprintf("%d", inv.Infra.TotalClusters), "vSphere clusters"})
	csvRows = append(csvRows, []string{"Total Hosts", fmt.Sprintf("%d", inv.Infra.TotalHosts), "ESXi hosts across all clusters"})

	// Additional infrastructure details...
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

	return csvRows
}

func (r *ReportService) addVMResourceDistribution(csvRows [][]string, inv v1alpha1.Inventory) [][]string {
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

func (r *ReportService) addResourceTotalsSummary(csvRows [][]string, inv v1alpha1.Inventory) [][]string {
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

func (r *ReportService) parseInventory(raw interface{}) (v1alpha1.Inventory, error) {
	var inv v1alpha1.Inventory

	switch data := raw.(type) {
	case []byte:
		err := json.Unmarshal(data, &inv)
		return inv, err
	case string:
		err := json.Unmarshal([]byte(data), &inv)
		return inv, err
	default:
		return inv, errors.New("unsupported inventory data type")
	}
}

func (r *ReportService) convertRowsToCSV(csvRows [][]string) (string, error) {
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

func (r *ReportService) generateEmptyCSVReport(source *model.Source) (string, error) {
	csvRows := [][]string{
		{"VMWARE INFRASTRUCTURE ASSESSMENT REPORT"},
		{fmt.Sprintf("Generated: %s at %s", time.Now().Format("01/02/2006"), time.Now().Format("3:04:05 PM"))},
		{""},
		{"NOTICE"},
		{""},
		{"No inventory data available for this source."},
		{"Please upload RVTools data or run discovery agent to populate inventory."},
		{""},
		{"Source Information", "Value"},
		{"Source Name", source.Name},
		{"Source ID", source.ID.String()},
		{"Created At", source.CreatedAt.Format(time.RFC3339)},
		{"On Premises", fmt.Sprintf("%v", source.OnPremises)},
	}

	return r.convertRowsToCSV(csvRows)
}

func (r *ReportService) generateEmptyHTMLReport(source *model.Source) (string, error) {
	data := EmptyReportTemplateData{
		CSS:           r.getInteractiveCSS(),
		GeneratedDate: time.Now().Format("1/2/2006"),
		GeneratedTime: time.Now().Format("3:04:05 PM"),
		SourceName:    source.Name,
		SourceID:      source.ID.String(),
		CreatedAt:     source.CreatedAt.Format(time.RFC3339),
		OnPremises:    source.OnPremises,
	}

	return r.executeTemplate(emptyReportTemplate, data)
}

func (r *ReportService) executeTemplate(templateStr string, data interface{}) (string, error) {
	tmpl, err := template.New("report").Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse HTML template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute HTML template: %w", err)
	}

	return buf.String(), nil
}

func (r *ReportService) getOSPriority(osName string) string {
	osLower := strings.ToLower(osName)
	switch {
	case strings.Contains(osLower, "windows"):
		return "High Priority"
	case strings.Contains(osLower, "linux"):
		return "Medium Priority"
	default:
		return "Review Required"
	}
}

func (r *ReportService) getImpactLevel(count int) string {
	switch {
	case count > 50:
		return "Critical"
	case count > 20:
		return "High"
	case count > 5:
		return "Medium"
	default:
		return "Low"
	}
}

func (r *ReportService) addHistogramSection(csvRows [][]string, config histogramConfig) [][]string {
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
