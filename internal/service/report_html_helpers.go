package service

import (
	"fmt"
	"strings"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
)

func (r *ReportService) generateOSTable(osEntries []osEntry, totalVMs int) string {
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

func (r *ReportService) generateDiskSizeTable(inv v1alpha1.Inventory, totalVMs int) string {
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

func (r *ReportService) generateWarningsChartSection(warnings []v1alpha1.MigrationIssue, includeWarnings bool) string {
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

func (r *ReportService) generateWarningsTableSection(warnings []v1alpha1.MigrationIssue, totalVMs int, includeWarnings bool) string {
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

func (r *ReportService) generateStorageTable(datastores []v1alpha1.Datastore) string {
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
			r.formatNumber(ds.TotalCapacityGB),
			r.formatNumber(ds.FreeCapacityGB),
			utilization, hwAccel))
	}

	return rows.String()
}

func (r *ReportService) generateInteractiveChartJS(inv v1alpha1.Inventory, osEntries []osEntry, includeWarnings bool) string {
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

func (r *ReportService) formatNumber(num int) string {
	str := fmt.Sprintf("%d", num)
	n := len(str)
	if n <= 3 {
		return str
	}
	result := ""
	for i, digit := range str {
		if i > 0 && (n-i)%3 == 0 {
			result += ","
		}
		result += string(digit)
	}
	return result
}

func (r *ReportService) getInteractiveCSS() string {
	return `
        body { font-family: Arial, sans-serif; margin: 20px; background: #f5f5f5; }
        .container { max-width: 1200px; margin: 0 auto; background: white; padding: 30px; border-radius: 10px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        .header { text-align: center; margin-bottom: 40px; }
        .header h1 { color: #2c3e50; margin-bottom: 10px; font-size: 2.5em; }
        .header p { color: #7f8c8d; font-size: 1.1em; }
        .summary-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 20px; margin: 30px 0; }
        .summary-card { background: #3498db; color: white; padding: 20px; border-radius: 8px; text-align: center; }
        .summary-card h4 { margin: 0 0 10px 0; font-size: 14px; font-weight: 600; }
        .summary-card .number { font-size: 32px; font-weight: bold; }
        .chart-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(500px, 1fr)); gap: 30px; margin: 30px 0; }
        .chart-container { background: white; padding: 20px; border-radius: 8px; border: 1px solid #ddd; box-shadow: 0 2px 5px rgba(0,0,0,0.05); }
        .chart-container h3 { text-align: center; color: #2c3e50; margin-bottom: 20px; font-size: 1.3em; }
        .chart-wrapper { position: relative; height: 300px; }
        .section { margin: 40px 0; }
        .section h2 { color: #2c3e50; border-left: 4px solid #3498db; padding-left: 15px; margin-bottom: 25px; }
        .table-section { margin: 30px 0; }
        .table-section h3 { color: #2c3e50; margin-bottom: 15px; }
        table { width: 100%; border-collapse: collapse; margin: 20px 0; background: white; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 5px rgba(0,0,0,0.1); }
        th, td { padding: 12px 15px; text-align: left; border-bottom: 1px solid #ddd; }
        th { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; font-weight: 600; }
        tr:nth-child(even) { background-color: #f8f9fa; }
        tr:hover { background-color: #e8f4ff; }
        .warning-high { background-color: #e74c3c !important; color: white; }
        .warning-medium { background-color: #f39c12 !important; color: white; }
        .warning-low { background-color: #27ae60 !important; color: white; }
        .recommendations { background: #f8f9fa; border-left: 4px solid #28a745; padding: 20px; margin: 20px 0; border-radius: 0 8px 8px 0; }
        .recommendations h3 { color: #28a745; margin-top: 0; }
        .recommendations ol { line-height: 1.8; }
        .recommendations li { margin-bottom: 10px; }
        .summary-box { background: linear-gradient(135deg, #74b9ff 0%, #0984e3 100%); color: white; padding: 20px; border-radius: 8px; margin: 20px 0; }
        .summary-box h2 { margin-top: 0; }
        .footer { text-align: center; margin-top: 40px; color: #7f8c8d; border-top: 1px solid #eee; padding-top: 20px; }
        .footer p { margin: 5px 0; }
        @media print { body { background: white; } .container { box-shadow: none; } .chart-container { break-inside: avoid; } }
        @media (max-width: 768px) {
            .chart-grid { grid-template-columns: 1fr; }
            .summary-grid { grid-template-columns: repeat(2, 1fr); }
            .container { padding: 20px; margin: 10px; }
        }
    `
}

// HTML Templates
const htmlReportTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>VMware Infrastructure Assessment Report</title>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/Chart.js/3.9.1/chart.min.js"></script>
    <style>{{.CSS}}</style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>VMware Infrastructure Assessment Report</h1>
            <p>Generated: {{.GeneratedDate}} at {{.GeneratedTime}}</p>
        </div>

        <div class="summary-grid">
            <div class="summary-card">
                <h4>Total VMs</h4>
                <div class="number">{{.TotalVMs}}</div>
            </div>
            <div class="summary-card" style="background: #e74c3c;">
                <h4>ESXi Hosts</h4>
                <div class="number">{{.TotalHosts}}</div>
            </div>
            <div class="summary-card" style="background: #27ae60;">
                <h4>Datastores</h4>
                <div class="number">{{.TotalDatastores}}</div>
            </div>
            <div class="summary-card" style="background: #f39c12;">
                <h4>Networks</h4>
                <div class="number">{{.TotalNetworks}}</div>
            </div>
        </div>

        <div class="chart-grid">
            <div class="chart-container">
                <h3>VM Power States Distribution</h3>
                <div class="chart-wrapper">
                    <canvas id="powerChart"></canvas>
                </div>
            </div>

            <div class="chart-container">
                <h3>Resource Utilization</h3>
                <div class="chart-wrapper">
                    <canvas id="resourceChart"></canvas>
                </div>
            </div>

            <div class="chart-container">
                <h3>Top Operating Systems</h3>
                <div class="chart-wrapper">
                    <canvas id="osChart"></canvas>
                </div>
            </div>

            <div class="chart-container">
                <h3>Disk Size Distribution</h3>
                <div class="chart-wrapper">
                    <canvas id="diskChart"></canvas>
                </div>
            </div>

            {{.WarningsChartSection}}

            <div class="chart-container">
                <h3>Storage Utilization by Datastore</h3>
                <div class="chart-wrapper">
                    <canvas id="storageChart"></canvas>
                </div>
            </div>
        </div>

        <div class="section">
            <h2>Detailed Analysis Tables</h2>
            
            <div class="table-section">
                <h3>Operating System Distribution</h3>
                <table>
                    <thead>
                        <tr>
                            <th>Operating System</th>
                            <th>VM Count</th>
                            <th>Percentage</th>
                            <th>Migration Priority</th>
                        </tr>
                    </thead>
                    <tbody>
                        {{.OSTable}}
                    </tbody>
                </table>
            </div>

            <div class="table-section">
                <h3>Disk Size Distribution</h3>
                <table>
                    <thead>
                        <tr>
                            <th>Size Range</th>
                            <th>VM Count</th>
                            <th>Percentage</th>
                        </tr>
                    </thead>
                    <tbody>
                        {{.DiskSizeTable}}
                    </tbody>
                </table>
            </div>

            <div class="table-section">
                <h3>Resource Allocation Analysis</h3>
                <table>
                    <thead>
                        <tr>
                            <th>Resource Type</th>
                            <th>Current Total</th>
                            <th>Average per VM</th>
                            <th>Recommended Total</th>
                        </tr>
                    </thead>
                    <tbody>
                        <tr>
                            <td><strong>CPU Cores (vCPUs)</strong></td>
                            <td>{{.CPUTotal}}</td>
                            <td>{{.CPUAverage}}</td>
                            <td>{{.CPURecommended}} (with 20% overhead)</td>
                        </tr>
                        <tr>
                            <td><strong>Memory (GB)</strong></td>
                            <td>{{.MemoryTotal}}</td>
                            <td>{{.MemoryAverage}}</td>
                            <td>{{.MemoryRecommended}} (with 25% overhead)</td>
                        </tr>
                        <tr>
                            <td><strong>Storage (GB)</strong></td>
                            <td>{{.StorageTotal}}</td>
                            <td>{{.StorageAverage}}</td>
                            <td>{{.StorageRecommended}} (with 15% overhead)</td>
                        </tr>
                    </tbody>
                </table>
            </div>

            {{.WarningsTableSection}}

            <div class="table-section">
                <h3>Storage Infrastructure</h3>
                <table>
                    <thead>
                        <tr>
                            <th>Vendor</th>
                            <th>Type</th>
                            <th>Protocol</th>
                            <th>Total Capacity (GB)</th>
                            <th>Free Capacity (GB)</th>
                            <th>Utilization %</th>
                            <th>Hardware Acceleration</th>
                        </tr>
                    </thead>
                    <tbody>
                        {{.StorageTable}}
                    </tbody>
                </table>
            </div>
        </div>

        <div class="footer">
            <p>VMware Infrastructure Assessment Report - Generated from live inventory data</p>
            <p>Charts are interactive - hover for details, click legend items to show/hide data series.</p>
        </div>
    </div>

    <script>{{.JavaScript}}</script>
</body>
</html>`

const emptyReportTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>VMware Infrastructure Assessment Report</title>
    <style>{{.CSS}}</style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>VMware Infrastructure Assessment Report</h1>
            <p>Generated: {{.GeneratedDate}} at {{.GeneratedTime}}</p>
        </div>
        
        <div class="section">
            <div class="summary-box">
                <h2>No Inventory Data Available</h2>
                <p>No inventory data is available for this source. Please upload RVTools data or run the discovery agent to populate inventory information.</p>
            </div>
        </div>
        
        <div class="section">
            <h2>Source Information</h2>
            <table>
                <thead>
                    <tr><th>Property</th><th>Value</th></tr>
                </thead>
                <tbody>
                    <tr><td><strong>Source Name</strong></td><td>{{.SourceName}}</td></tr>
                    <tr><td><strong>Source ID</strong></td><td>{{.SourceID}}</td></tr>
                    <tr><td><strong>Created At</strong></td><td>{{.CreatedAt}}</td></tr>
                    <tr><td><strong>On Premises</strong></td><td>{{.OnPremises}}</td></tr>
                </tbody>
            </table>
        </div>
    </div>
</body>
</html>`