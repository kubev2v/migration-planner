package html

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/service/report/types"
)

type Renderer struct{}

type osEntry struct {
	name  string
	count int
}

func NewRenderer() *Renderer {
	return &Renderer{}
}

func (r *Renderer) SupportedFormat() types.ReportFormat {
	return types.ReportFormatHTML
}

func (r *Renderer) Render(data *types.ReportData) (string, error) {
	if data.Inventory == nil {
		return r.generateEmptyReport(data)
	}

	osEntries := make([]osEntry, len(data.Metrics.OSDetails))
	for i, os := range data.Metrics.OSDetails {
		osEntries[i] = osEntry{name: os.Name, count: os.Count}
	}

	templateData := types.ReportTemplateData{
		CSS:           r.getInteractiveCSS(),
		GeneratedDate: data.Timestamps.Generated,
		GeneratedTime: data.Timestamps.GeneratedTime,

		TotalVMs:        data.Metrics.Executive.TotalVMs,
		TotalHosts:      data.Metrics.Executive.TotalHosts,
		TotalDatastores: data.Metrics.Executive.TotalDatastores,
		TotalNetworks:   data.Metrics.Executive.TotalNetworks,

		OSTable:              r.generateOSTable(osEntries, data.Metrics.Executive.TotalVMs),
		DiskSizeTable:        r.generateDiskSizeTable(data.Inventory, data.Metrics.Executive.TotalVMs),
		StorageTable:         r.generateStorageTable(data.Inventory.Infra.Datastores),
		WarningsTableSection: r.generateWarningsTableSection(data.Inventory.Vms.MigrationWarnings, data.Metrics.Executive.TotalVMs, data.Options.IncludeWarnings),
		WarningsChartSection: r.generateWarningsChartSection(data.Inventory.Vms.MigrationWarnings, data.Options.IncludeWarnings),

		CPUTotal:           data.Metrics.Resources.CPU.Total,
		CPUAverage:         fmt.Sprintf("%.1f", data.Metrics.Resources.CPU.Average),
		CPURecommended:     data.Metrics.Resources.CPU.Recommended,
		MemoryTotal:        data.Metrics.Resources.Memory.Total,
		MemoryAverage:      fmt.Sprintf("%.1f", data.Metrics.Resources.Memory.Average),
		MemoryRecommended:  data.Metrics.Resources.Memory.Recommended,
		StorageTotal:       data.Metrics.Resources.Disk.Total,
		StorageAverage:     fmt.Sprintf("%.1f", data.Metrics.Resources.Disk.Average),
		StorageRecommended: data.Metrics.Resources.Disk.Recommended,

		JavaScript: r.generateInteractiveChartJS(data.Inventory, osEntries, data.Options.IncludeWarnings,
			data.Metrics.Resources.CPU.Total, data.Metrics.Resources.CPU.Recommended,
			data.Metrics.Resources.Memory.Total, data.Metrics.Resources.Memory.Recommended,
			data.Metrics.Resources.Disk.Total, data.Metrics.Resources.Disk.Recommended),
	}

	return r.executeTemplate(htmlReportTemplate, templateData)
}

func (r *Renderer) generateEmptyReport(data *types.ReportData) (string, error) {
	templateData := types.EmptyReportTemplateData{
		CSS:           r.getInteractiveCSS(),
		GeneratedDate: data.Timestamps.Generated,
		GeneratedTime: data.Timestamps.GeneratedTime,
		SourceName:    data.Source.Name,
		SourceID:      data.Source.ID.String(),
		CreatedAt:     data.Source.CreatedAt.Format(time.RFC3339),
		OnPremises:    data.Source.OnPremises,
	}

	return r.executeTemplate(emptyReportTemplate, templateData)
}

func (r *Renderer) executeTemplate(templateStr string, data interface{}) (string, error) {
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

func (r *Renderer) generateOSTable(osEntries []osEntry, totalVMs int) string {
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

func (r *Renderer) generateDiskSizeTable(inv *v1alpha1.Inventory, totalVMs int) string {
	if len(inv.Vms.DiskGB.Histogram.Data) == 0 || totalVMs == 0 {
		return `<tr><td colspan="3">No disk size data available</td></tr>`
	}

	var rows strings.Builder
	minVal := inv.Vms.DiskGB.Histogram.MinValue
	step := inv.Vms.DiskGB.Histogram.Step

	for i, count := range inv.Vms.DiskGB.Histogram.Data {
		if count > 0 {
			rangeStart := minVal + (i * step)
			rangeEnd := rangeStart + step - 1
			label := fmt.Sprintf("%d-%d GB", rangeStart, rangeEnd)
			percentage := fmt.Sprintf("%.1f", (float64(count)/float64(totalVMs))*100)

			rows.WriteString(fmt.Sprintf(`
                        <tr>
                            <td><strong>%s</strong></td>
                            <td>%d</td>
                            <td>%s%%</td>
                        </tr>`, label, count, percentage))
		}
	}

	return rows.String()
}

func (r *Renderer) generateWarningsChartSection(warnings []v1alpha1.MigrationIssue, includeWarnings bool) string {
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

func (r *Renderer) generateWarningsTableSection(warnings []v1alpha1.MigrationIssue, totalVMs int, includeWarnings bool) string {
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

func (r *Renderer) generateStorageTable(datastores []v1alpha1.Datastore) string {
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

func (r *Renderer) generateInteractiveChartJS(inv *v1alpha1.Inventory, osEntries []osEntry, includeWarnings bool, cpuTotal, cpuRecommended, memoryTotal, memoryRecommended, storageTotal, storageRecommended int) string {
	// Power States Chart
	powerStatesJS := r.generatePowerStatesChart(inv)

	// Resource Utilization Chart
	resourceUtilJS := r.generateResourceUtilizationChart(cpuTotal, cpuRecommended, memoryTotal, memoryRecommended, storageTotal, storageRecommended)

	// Operating Systems Chart - Top 8 OS entries
	osChartJS := r.generateOSChart(osEntries)

	// Disk Size Distribution Chart
	diskSizeJS := r.generateDiskSizeChart(inv)

	// Warnings Chart
	warningsJS := ""
	if includeWarnings && len(inv.Vms.MigrationWarnings) > 0 {
		warningsJS = r.generateWarningsChart(inv.Vms.MigrationWarnings)
	}

	// Storage Utilization Chart
	storageJS := r.generateStorageChart(inv.Infra.Datastores)

	return fmt.Sprintf(`
        <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
        <script>
            document.addEventListener('DOMContentLoaded', function() {
                %s
                %s
                %s
                %s
                %s
                %s
            });
        </script>
    `, powerStatesJS, resourceUtilJS, osChartJS, diskSizeJS, warningsJS, storageJS)
}

func (r *Renderer) generatePowerStatesChart(inv *v1alpha1.Inventory) string {

	poweredOn := 0
	poweredOff := 0
	suspended := 0

	if states, exists := inv.Vms.PowerStates["poweredOn"]; exists {
		poweredOn = states
	}
	if states, exists := inv.Vms.PowerStates["poweredOff"]; exists {
		poweredOff = states
	}
	if states, exists := inv.Vms.PowerStates["suspended"]; exists {
		suspended = states
	}

	return fmt.Sprintf(`
        // Power States Doughnut Chart
        const powerCtx = document.getElementById('powerChart');
        if (powerCtx) {
            new Chart(powerCtx, {
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
        }`, poweredOn, poweredOff, suspended)
}

func (r *Renderer) generateResourceUtilizationChart(cpuTotal, cpuRecommended, memoryTotal, memoryRecommended, storageTotal, storageRecommended int) string {
	return fmt.Sprintf(`
        // Resource Utilization Bar Chart
        const resourceCtx = document.getElementById('resourceChart');
        if (resourceCtx) {
            new Chart(resourceCtx, {
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
        }`,
		cpuTotal, memoryTotal, storageTotal,
		cpuRecommended, memoryRecommended, storageRecommended)
}

func (r *Renderer) generateOSChart(osEntries []osEntry) string {
	if len(osEntries) == 0 {
		return `
        // Operating Systems Chart - No data available
        const osCtx = document.getElementById('osChart');
        if (osCtx) {
            new Chart(osCtx, {
                type: 'bar',
                data: {
                    labels: ["No Data Available"],
                    datasets: [{
                        data: [0],
                        backgroundColor: ["#cccccc"]
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
        }`
	}

	// Take top 8 OS entries
	topOSEntries := osEntries
	if len(topOSEntries) > 8 {
		topOSEntries = topOSEntries[:8]
	}

	var labels []string
	var data []int
	colors := []string{`"#3498db"`, `"#e74c3c"`, `"#27ae60"`, `"#f39c12"`, `"#9b59b6"`, `"#1abc9c"`, `"#34495e"`, `"#e67e22"`}

	for i, os := range topOSEntries {
		labels = append(labels, fmt.Sprintf(`"%s"`, os.name))
		data = append(data, os.count)
		if i >= 7 {
			break
		}
	}

	return fmt.Sprintf(`
        // Operating Systems Horizontal Bar Chart
        const osCtx = document.getElementById('osChart');
        if (osCtx) {
            new Chart(osCtx, {
                type: 'bar',
                data: {
                    labels: [%s],
                    datasets: [{
                        data: [%s],
                        backgroundColor: [%s]
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
        }`,
		strings.Join(labels, ","),
		strings.Join(r.intSliceToStringSlice(data), ","),
		strings.Join(colors[:len(data)], ","))
}

func (r *Renderer) generateDiskSizeChart(inv *v1alpha1.Inventory) string {
	var diskRanges []string
	var diskCounts []int

	if len(inv.Vms.DiskGB.Histogram.Data) > 0 {
		minVal := inv.Vms.DiskGB.Histogram.MinValue
		step := inv.Vms.DiskGB.Histogram.Step
		for i, count := range inv.Vms.DiskGB.Histogram.Data {
			if count > 0 {
				rangeStart := minVal + (i * step)
				rangeEnd := rangeStart + step - 1
				diskRanges = append(diskRanges, fmt.Sprintf("'%d-%d GB'", rangeStart, rangeEnd))
				diskCounts = append(diskCounts, count)
			}
		}
	}

	if len(diskRanges) == 0 {
		diskRanges = append(diskRanges, "'No Data'")
		diskCounts = append(diskCounts, 0)
	}

	return fmt.Sprintf(`
        // Disk Distribution Chart
        const diskCtx = document.getElementById('diskChart');
        if (diskCtx) {
            new Chart(diskCtx, {
                type: 'bar',
                data: {
                    labels: [%s],
                    datasets: [{
                        label: 'Number of VMs',
                        data: [%s],
                        backgroundColor: '#4BC0C0'
                    }]
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    plugins: {
                        legend: {
                            display: false
                        }
                    }
                }
            });
        }`,
		strings.Join(diskRanges, ", "),
		strings.Join(r.intSliceToStringSlice(diskCounts), ", "))
}

func (r *Renderer) generateWarningsChart(warnings []v1alpha1.MigrationIssue) string {
	var labels []string
	var data []int
	var colors []string

	for _, warning := range warnings {
		labels = append(labels, fmt.Sprintf(`"%s"`, warning.Label))
		data = append(data, warning.Count)

		// Color based on impact
		if warning.Count > 50 {
			colors = append(colors, `"#e74c3c"`) // Critical - red
		} else if warning.Count > 20 {
			colors = append(colors, `"#f39c12"`) // High - orange
		} else if warning.Count > 5 {
			colors = append(colors, `"#27ae60"`) // Medium - green
		} else {
			colors = append(colors, `"#3498db"`) // Low - blue
		}
	}

	return fmt.Sprintf(`
        // Migration Warnings Chart
        const warningsCtx = document.getElementById('warningsChart');
        if (warningsCtx) {
            new Chart(warningsCtx, {
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
            });
        }`,
		strings.Join(labels, ","),
		strings.Join(r.intSliceToStringSlice(data), ","),
		strings.Join(colors, ","))
}

func (r *Renderer) generateStorageChart(datastores []v1alpha1.Datastore) string {
	var labels []string
	var usedData []int
	var totalData []int

	for _, ds := range datastores {
		label := fmt.Sprintf(`"%s %s"`, ds.Vendor, ds.Type)
		labels = append(labels, label)
		usedData = append(usedData, ds.TotalCapacityGB-ds.FreeCapacityGB)
		totalData = append(totalData, ds.TotalCapacityGB)
	}

	return fmt.Sprintf(`
        // Storage Utilization Chart
        const storageCtx = document.getElementById('storageChart');
        if (storageCtx) {
            new Chart(storageCtx, {
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
            });
        }`,
		strings.Join(labels, ","),
		strings.Join(r.intSliceToStringSlice(usedData), ","),
		strings.Join(r.intSliceToStringSlice(totalData), ","))
}

func (r *Renderer) formatNumber(num int) string {
	if num >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(num)/1000000)
	} else if num >= 1000 {
		return fmt.Sprintf("%.1fK", float64(num)/1000)
	}
	return fmt.Sprintf("%d", num)
}

func (r *Renderer) intSliceToStringSlice(ints []int) []string {
	result := make([]string, len(ints))
	for i, v := range ints {
		result[i] = fmt.Sprintf("%d", v)
	}
	return result
}

func (r *Renderer) getInteractiveCSS() string {
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
        }`
}

const htmlReportTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>VMware Infrastructure Assessment Report</title>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/Chart.js/3.9.1/chart.min.js"></script>
    <style>
        {{.CSS}}
    </style>
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

    {{.JavaScript}}
</body>
</html>`

const emptyReportTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>VMware Infrastructure Assessment Report</title>
    <style>
        {{.CSS}}
    </style>
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