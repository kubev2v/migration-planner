// template.go - HTML templates for reports
package reports

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

// Template data structures
type ReportTemplateData struct {
    // Header data
    CSS           string
    GeneratedDate string
    GeneratedTime string
    
    // Summary cards
    TotalVMs        int
    TotalHosts      int
    TotalDatastores int
    TotalNetworks   int
    
    // Table content
    OSTable              string
    DiskSizeTable        string
    StorageTable         string
    WarningsTableSection string
    WarningsChartSection string
    
    // Resource data
    CPUTotal          int
    CPUAverage        string
    CPURecommended    int
    MemoryTotal       int
    MemoryAverage     string
    MemoryRecommended int
    StorageTotal      int
    StorageAverage    string
    StorageRecommended int
    
    // JavaScript
    JavaScript string
}

type EmptyReportTemplateData struct {
    CSS           string
    GeneratedDate string
    GeneratedTime string
    SourceName    string
    SourceID      string
    CreatedAt     string
    OnPremises    bool
}