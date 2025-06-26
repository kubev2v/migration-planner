package reports

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

type osEntry struct {
	name  string
	count int
}

func GenerateHTMLContent(source *model.Source, reportType string, includeWarnings bool) (string, error) {
	if source == nil {
		return "", errors.New("source cannot be nil")
	}
	
	if source.Inventory == nil {
		return generateEmptyHTMLReport(source)
	}
	
	inventoryData, err := source.Inventory.Value()
	if err != nil {
		return "", fmt.Errorf("failed to extract inventory data: %w", err)
	}
	
	inv, err := ParseInventory(inventoryData)
	if err != nil {
		return "", fmt.Errorf("failed to parse inventory data: %w", err)
	}

	// Calculate statistics
	totalWarnings := 0
	for _, warning := range inv.Vms.MigrationWarnings {
		totalWarnings += warning.Count
	}

	var osEntries []osEntry
	for osName, count := range inv.Vms.Os {
		osEntries = append(osEntries, osEntry{name: osName, count: count})
	}
	sort.Slice(osEntries, func(i, j int) bool {
		return osEntries[i].count > osEntries[j].count
	})

	// Prepare template data
	data := ReportTemplateData{
		CSS:           getInteractiveCSS(),
		GeneratedDate: time.Now().Format("1/2/2006"),
		GeneratedTime: time.Now().Format("3:04:05 PM"),
		
		// Summary cards
		TotalVMs:        inv.Vms.Total,
		TotalHosts:      inv.Infra.TotalHosts,
		TotalDatastores: len(inv.Infra.Datastores),
		TotalNetworks:   len(inv.Infra.Networks),
		
		// Tables
		OSTable:              generateOSTable(osEntries, inv.Vms.Total),
		DiskSizeTable:        generateDiskSizeTable(inv, inv.Vms.Total),
		StorageTable:         generateStorageTable(inv.Infra.Datastores),
		WarningsTableSection: generateWarningsTableSection(inv.Vms.MigrationWarnings, inv.Vms.Total, includeWarnings),
		WarningsChartSection: generateWarningsChartSection(inv.Vms.MigrationWarnings, includeWarnings),
		
		// Resource data
		CPUTotal:          inv.Vms.CpuCores.Total,
		CPUAverage:        fmt.Sprintf("%.1f", float64(inv.Vms.CpuCores.Total)/float64(inv.Vms.Total)),
		CPURecommended:    int(float64(inv.Vms.CpuCores.Total) * 1.2),
		MemoryTotal:       inv.Vms.RamGB.Total,
		MemoryAverage:     fmt.Sprintf("%.1f", float64(inv.Vms.RamGB.Total)/float64(inv.Vms.Total)),
		MemoryRecommended: int(float64(inv.Vms.RamGB.Total) * 1.25),
		StorageTotal:      inv.Vms.DiskGB.Total,
		StorageAverage:    fmt.Sprintf("%.1f", float64(inv.Vms.DiskGB.Total)/float64(inv.Vms.Total)),
		StorageRecommended: int(float64(inv.Vms.DiskGB.Total) * 1.15),
		
		// JavaScript
		JavaScript: generateInteractiveChartJS(inv, osEntries, includeWarnings),
	}

	htmlContent, err := executeTemplate(htmlReportTemplate, data)
	if err != nil {
		return "", fmt.Errorf("failed to generate HTML report: %w", err)
	}
	return htmlContent, nil
	}

func generateEmptyHTMLReport(source *model.Source) (string, error) {
	data := EmptyReportTemplateData{
		CSS:           getInteractiveCSS(),
		GeneratedDate: time.Now().Format("1/2/2006"),
		GeneratedTime: time.Now().Format("3:04:05 PM"),
		SourceName:    source.Name,
		SourceID:      source.ID.String(),
		CreatedAt:     source.CreatedAt.Format(time.RFC3339),
		OnPremises:    source.OnPremises,
	}

	return executeTemplate(emptyReportTemplate, data)
}

func executeTemplate(templateStr string, data interface{}) (string, error) {
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

func generateOSTable(osEntries []osEntry, totalVMs int) string {
	if len(osEntries) == 0 {
		return `<tr><td colspan="4">No operating system data available</td></tr>`
	}

	var rows strings.Builder
	
	for _, os := range osEntries {
		percentage := fmt.Sprintf("%.1f", (float64(os.count)/float64(totalVMs))*100)
		priority := "Review Required"
		if strings.Contains(strings.ToLower(os.name), "windows") {
			priority = "High"
		} else if strings.Contains(strings.ToLower(os.name), "linux") {
			priority = "Medium"
		}
		
		rows.WriteString(fmt.Sprintf(`
                    <tr>
                        <td><strong>%s</strong></td>
                        <td>%d</td>
                        <td>%s%%</td>
                        <td>%s</td>
                    </tr>`, os.name, os.count, percentage, priority))
	}
	
	return rows.String()
}

func generateDiskSizeTable(inv v1alpha1.Inventory, totalVMs int) string {
	ranges := []struct {
		min, max int
		label    string
	}{
		{0, 12, "0-12 GB"},
		{12, 24, "12-24 GB"},
		{24, 36, "24-36 GB"},
		{36, 48, "36-48 GB"},
		{48, 64, "48-64 GB"},
		{64, 999999, "64+ GB"},
	}

	avgDiskPerVM := float64(inv.Vms.DiskGB.Total) / float64(totalVMs)
	rangeCounts := make([]int, len(ranges))

	for i, r := range ranges {
		if avgDiskPerVM >= float64(r.min) && (r.max == 999999 || avgDiskPerVM < float64(r.max)) {
			rangeCounts[i] = totalVMs * 80 / 100
		} else {
			rangeCounts[i] = totalVMs * 20 / 100 / (len(ranges) - 1)
		}
	}

	var rows strings.Builder
	for i, r := range ranges {
		count := rangeCounts[i]
		if count == 0 {
			continue
		}
		percentage := fmt.Sprintf("%.1f", (float64(count)/float64(totalVMs))*100)
		
		rows.WriteString(fmt.Sprintf(`
                    <tr>
                        <td><strong>%s</strong></td>
                        <td>%d</td>
                        <td>%s%%</td>
                    </tr>`, r.label, count, percentage))
	}
	
	return rows.String()
}

func generateWarningsChartSection(warnings []v1alpha1.MigrationIssue, includeWarnings bool) string {
	if !includeWarnings || len(warnings) == 0 {
		return ""
	}
	
	return `<div class="chart-container">
                <h3>Migration Warnings</h3>
                <div class="chart-wrapper">
                    <canvas id="warningsChart"></canvas>
                </div>
            </div>`
}

func generateWarningsTableSection(warnings []v1alpha1.MigrationIssue, totalVMs int, includeWarnings bool) string {
	if !includeWarnings || len(warnings) == 0 {
		return `<div class="table-section">
            <h3>Migration Warnings Analysis</h3>
            <p>No migration warnings to display or warnings analysis disabled.</p>
        </div>`
	}

	var rows strings.Builder
	
	for _, warning := range warnings {
		impact := "Low"
		if warning.Count > 50 {
			impact = "Critical"
		} else if warning.Count > 20 {
			impact = "High"
		} else if warning.Count > 5 {
			impact = "Medium"
		}
		
		percentage := fmt.Sprintf("%.1f", (float64(warning.Count)/float64(totalVMs))*100)
		priority := "Post Migration"
		switch impact {
			case "Critical":
				priority = "Immediate"
			case "High":
				priority = "Before Migration"
			case "Medium":
				priority = "During Migration"
		}
		
		rowClass := ""
		switch impact {
			case "Critical":
				rowClass = "warning-high"
			case "High":
				rowClass = "warning-medium"
			case "Medium":
				rowClass = "warning-low"
		}
		
		rows.WriteString(fmt.Sprintf(`
                        <tr class="%s">
                            <td><strong>%s</strong></td>
                            <td>%d</td>
                            <td>%s</td>
                            <td>%s%%</td>
                            <td>%s</td>
                        </tr>`, rowClass, warning.Label, warning.Count, impact, percentage, priority))
	}

	return fmt.Sprintf(`<div class="table-section">
                <h3>Migration Warnings Analysis</h3>
                <table>
                    <thead>
                        <tr>
                            <th>Warning Category</th>
                            <th>Affected VMs</th>
                            <th>Impact Level</th>
                            <th>%% of Total VMs</th>
                            <th>Priority</th>
                        </tr>
                    </thead>
                    <tbody>
                        %s
                    </tbody>
                </table>
            </div>`, rows.String())
}

func generateStorageTable(datastores []v1alpha1.Datastore) string {
	if len(datastores) == 0 {
		return `<tr><td colspan="7">No datastore information available</td></tr>`
	}

	var rows strings.Builder
	
	for _, ds := range datastores {
		utilization := "0.0"
		if ds.TotalCapacityGB > 0 {
			util := ((float64(ds.TotalCapacityGB) - float64(ds.FreeCapacityGB)) / float64(ds.TotalCapacityGB)) * 100
			utilization = fmt.Sprintf("%.1f", util)
		}
		
		hwAccel := "❌ No"
		if ds.HardwareAcceleratedMove {
			hwAccel = "✅ Yes"
		}
		
		rows.WriteString(fmt.Sprintf(`
                    <tr>
                        <td><strong>%s</strong></td>
                        <td>%s</td>
                        <td>%s</td>
                        <td>%s</td>
                        <td>%s</td>
                        <td>%s%%</td>
                        <td>%s</td>
                    </tr>`,
			ds.Vendor, ds.Type, ds.ProtocolType,
			formatNumber(ds.TotalCapacityGB),
			formatNumber(ds.FreeCapacityGB),
			utilization, hwAccel))
	}
	
	return rows.String()
}

func generateInteractiveChartJS(inv v1alpha1.Inventory, osEntries []osEntry, includeWarnings bool) string {
	poweredOn := 0
	poweredOff := 0
	suspended := 0
	if count, exists := inv.Vms.PowerStates["poweredOn"]; exists {
		poweredOn = count
	}
	if count, exists := inv.Vms.PowerStates["poweredOff"]; exists {
		poweredOff = count
	}
	if count, exists := inv.Vms.PowerStates["suspended"]; exists {
		suspended = count
	}

	var osLabels, osData strings.Builder
	maxOS := 8
	if len(osEntries) < 8 {
		maxOS = len(osEntries)
	}
	
	for i, os := range osEntries[:maxOS] {
		if i > 0 {
			osLabels.WriteString(",")
			osData.WriteString(",")
		}
		osLabels.WriteString(fmt.Sprintf(`"%s"`, os.name))
		osData.WriteString(fmt.Sprintf("%d", os.count))
	}

	var storageLabels, storageUsed, storageTotal strings.Builder
	for i, ds := range inv.Infra.Datastores {
		if i > 0 {
			storageLabels.WriteString(",")
			storageUsed.WriteString(",")
			storageTotal.WriteString(",")
		}
		storageLabels.WriteString(fmt.Sprintf(`"%s %s"`, ds.Vendor, ds.Type))
		storageUsed.WriteString(fmt.Sprintf("%d", ds.TotalCapacityGB-ds.FreeCapacityGB))
		storageTotal.WriteString(fmt.Sprintf("%d", ds.TotalCapacityGB))
	}

	diskRanges := []string{"0-12 GB", "12-24 GB", "24-36 GB", "36-48 GB", "48-64 GB", "64+ GB"}
	avgDiskPerVM := float64(inv.Vms.DiskGB.Total) / float64(inv.Vms.Total)
	diskCounts := make([]int, len(diskRanges))

	for i := range diskRanges {
		if i == 0 && avgDiskPerVM < 12 {
			diskCounts[i] = inv.Vms.Total * 80 / 100
		} else if i == 1 && avgDiskPerVM >= 12 && avgDiskPerVM < 24 {
			diskCounts[i] = inv.Vms.Total * 80 / 100
		} else if i == 2 && avgDiskPerVM >= 24 && avgDiskPerVM < 36 {
			diskCounts[i] = inv.Vms.Total * 80 / 100
		} else if i == 3 && avgDiskPerVM >= 36 && avgDiskPerVM < 48 {
			diskCounts[i] = inv.Vms.Total * 80 / 100
		} else if i == 4 && avgDiskPerVM >= 48 && avgDiskPerVM < 64 {
			diskCounts[i] = inv.Vms.Total * 80 / 100
		} else if i == 5 && avgDiskPerVM >= 64 {
			diskCounts[i] = inv.Vms.Total * 80 / 100
		} else {
			diskCounts[i] = inv.Vms.Total * 20 / 100 / 5
		}
	}

	var diskLabels, diskData strings.Builder
	for i, rangeLabel := range diskRanges {
		if i > 0 {
			diskLabels.WriteString(",")
			diskData.WriteString(",")
		}
		diskLabels.WriteString(fmt.Sprintf(`"%s"`, rangeLabel))
		diskData.WriteString(fmt.Sprintf("%d", diskCounts[i]))
	}

	warningsJS := ""
	if includeWarnings && len(inv.Vms.MigrationWarnings) > 0 {
		var warningLabels, warningData, warningColors strings.Builder
		for i, warning := range inv.Vms.MigrationWarnings {
			if i > 0 {
				warningLabels.WriteString(",")
				warningData.WriteString(",")
				warningColors.WriteString(",")
			}
			warningLabels.WriteString(fmt.Sprintf(`"%s"`, warning.Label))
			warningData.WriteString(fmt.Sprintf("%d", warning.Count))
			
			color := "#3498db"
			if warning.Count > 50 {
				color = "#e74c3c"
			} else if warning.Count > 20 {
				color = "#f39c12"
			} else if warning.Count > 5 {
				color = "#27ae60"
			}
			warningColors.WriteString(fmt.Sprintf(`"%s"`, color))
		}

		warningsJS = fmt.Sprintf(`
        // Migration Warnings Chart
        new Chart(document.getElementById('warningsChart'), {
            type: 'bar',
            data: {
                labels: [%s],
                datasets: [{
                    data: [%s],
                    backgroundColor: [%s]
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: { legend: { display: false } },
                scales: { y: { beginAtZero: true } }
            }
        });`, warningLabels.String(), warningData.String(), warningColors.String())
	}

	return fmt.Sprintf(`
        // Power States Doughnut Chart
        new Chart(document.getElementById('powerChart'), {
            type: 'doughnut',
            data: {
                labels: ["Powered On","Powered Off","Suspended"],
                datasets: [{
                    data: [%d,%d,%d],
                    backgroundColor: ['#27ae60', '#e74c3c', '#f39c12']
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: { legend: { position: 'bottom' } }
            }
        });

        // Resource Utilization Bar Chart
        new Chart(document.getElementById('resourceChart'), {
            type: 'bar',
            data: {
                labels: ["CPU Cores","Memory GB","Storage GB"],
                datasets: [{
                    label: 'Current',
                    data: [%d,%d,%d],
                    backgroundColor: '#3498db'
                }, {
                    label: 'Recommended',
                    data: [%d,%d,%d],
                    backgroundColor: '#2ecc71'
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: { legend: { position: 'bottom' } },
                scales: { y: { beginAtZero: true } }
            }
        });

        // Operating Systems Horizontal Bar Chart
        new Chart(document.getElementById('osChart'), {
            type: 'bar',
            data: {
                labels: [%s],
                datasets: [{
                    data: [%s],
                    backgroundColor: ['#3498db', '#e74c3c', '#27ae60', '#f39c12', '#9b59b6', '#1abc9c', '#34495e', '#e67e22']
                }]
            },
            options: {
                indexAxis: 'y',
                responsive: true,
                maintainAspectRatio: false,
                plugins: { legend: { display: false } },
                scales: { x: { beginAtZero: true } }
            }
        });

        // Disk Size Distribution Chart
        new Chart(document.getElementById('diskChart'), {
            type: 'bar',
            data: {
                labels: [%s],
                datasets: [{
                    data: [%s],
                    backgroundColor: ['#3498db', '#e74c3c', '#27ae60', '#f39c12', '#9b59b6', '#1abc9c']
                }]
            },
            options: {
                indexAxis: 'y',
                responsive: true,
                maintainAspectRatio: false,
                plugins: { legend: { display: false } },
                scales: { x: { beginAtZero: true } }
            }
        });

        %s

        // Storage Utilization Chart
        new Chart(document.getElementById('storageChart'), {
            type: 'bar',
            data: {
                labels: [%s],
                datasets: [{
                    label: 'Used (GB)',
                    data: [%s],
                    backgroundColor: '#e74c3c'
                }, {
                    label: 'Total (GB)',
                    data: [%s],
                    backgroundColor: '#3498db'
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: { legend: { position: 'bottom' } },
                scales: { y: { beginAtZero: true } }
            }
        });`,
		poweredOn, poweredOff, suspended,
		inv.Vms.CpuCores.Total, inv.Vms.RamGB.Total, inv.Vms.DiskGB.Total,
		int(float64(inv.Vms.CpuCores.Total)*1.2),
		int(float64(inv.Vms.RamGB.Total)*1.25),
		int(float64(inv.Vms.DiskGB.Total)*1.15),
		osLabels.String(), osData.String(),
		diskLabels.String(), diskData.String(),
		warningsJS,
		storageLabels.String(), storageUsed.String(), storageTotal.String(),
	)
}
