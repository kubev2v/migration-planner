package reports

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/store/model"
)
type ReportMetadata struct {
	Title     string
	Generated string
	Source    *model.Source
}

func ParseInventory(raw interface{}) (v1alpha1.Inventory, error) {
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

func getReportMetadata(source *model.Source) ReportMetadata {
	return ReportMetadata{
		Title:     "VMWARE INFRASTRUCTURE ASSESSMENT REPORT",
		Generated: fmt.Sprintf("Generated: %s at %s", time.Now().Format("01/02/2006"), time.Now().Format("3:04:05 PM")),
		Source:    source,
	}
}

func GenerateEmptyCSVReport(meta ReportMetadata) (string, error) {
	csvRows := [][]string{
		{meta.Title},
		{meta.Generated},
		{""},
		{"NOTICE"},
		{""},
		{"No inventory data available for this source."},
		{"Please upload RVTools data or run discovery agent to populate inventory."},
		{""},
		{"Source Information", "Value"},
		{"Source Name", meta.Source.Name},
		{"Source ID", meta.Source.ID.String()},
		{"Created At", meta.Source.CreatedAt.Format(time.RFC3339)},
		{"On Premises", fmt.Sprintf("%v", meta.Source.OnPremises)},
	}
	
	csvContent, err := convertRowsToCSV(csvRows)
	if err != nil {
		return "", fmt.Errorf("failed to generate empty CSV report: %w", err)
	}
	
	return csvContent, nil
}

func convertRowsToCSV(csvRows [][]string) (string, error) {
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

func getOSPriority(osName string) string {
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

func getImpactLevel(count int) string {
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

type histogramConfig struct {
    histogram   v1alpha1.Histogram
    title       string
    unit        string
    noDataMsg   string
}

func addHistogramSection(csvRows *[][]string, config histogramConfig) {
    *csvRows = append(*csvRows, []string{config.title})
    *csvRows = append(*csvRows, []string{"Range", "VM Count"})
    
    if len(config.histogram.Data) > 0 {
        minVal := config.histogram.MinValue
        step := config.histogram.Step
        for i, count := range config.histogram.Data {
            if count > 0 { // Only show ranges with VMs
                rangeStart := minVal + (i * step)
                rangeEnd := rangeStart + step - 1
                *csvRows = append(*csvRows, []string{
                    fmt.Sprintf("%d-%d %s", rangeStart, rangeEnd, config.unit),
                    fmt.Sprintf("%d", count),
                })
            }
        }
    } else {
        *csvRows = append(*csvRows, []string{config.noDataMsg, "0"})
    }
    *csvRows = append(*csvRows, []string{""})
}

func formatNumber(num int) string {
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

func getInteractiveCSS() string {
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

func GenerateEmptyHTMLReport(meta ReportMetadata) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%s</title>
    <style>%s</style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>%s</h1>
            <p>%s</p>
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
                    <tr><td><strong>Source Name</strong></td><td>%s</td></tr>
                    <tr><td><strong>Source ID</strong></td><td>%s</td></tr>
                    <tr><td><strong>Created At</strong></td><td>%s</td></tr>
                    <tr><td><strong>On Premises</strong></td><td>%v</td></tr>
                </tbody>
            </table>
        </div>
    </div>
</body>
</html>`,
		meta.Title,
		getInteractiveCSS(),
		meta.Title,
		meta.Generated,
		meta.Source.Name,
		meta.Source.ID.String(),
		meta.Source.CreatedAt.Format(time.RFC3339),
		meta.Source.OnPremises)
}
