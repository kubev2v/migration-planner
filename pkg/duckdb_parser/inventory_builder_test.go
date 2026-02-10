package duckdb_parser

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/marcboeker/go-duckdb/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"github.com/kubev2v/migration-planner/pkg/duckdb_parser/models"
)

// testValidator returns no concerns for all VMs.
type testValidator struct{}

func (v *testValidator) Validate(ctx context.Context, vm models.VM) ([]models.Concern, error) {
	return nil, nil
}

// testWarningValidator returns warnings for all VMs.
type testWarningValidator struct{}

func (v *testWarningValidator) Validate(ctx context.Context, vm models.VM) ([]models.Concern, error) {
	return []models.Concern{
		{
			Id:         "test.warning",
			Label:      "Test Warning",
			Category:   "Warning",
			Assessment: "This is a test warning for testing purposes.",
		},
	}, nil
}

// testCriticalValidator returns critical concerns for all VMs.
type testCriticalValidator struct{}

func (v *testCriticalValidator) Validate(ctx context.Context, vm models.VM) ([]models.Concern, error) {
	return []models.Concern{
		{
			Id:         "test.critical",
			Label:      "Test Critical",
			Category:   "Critical",
			Assessment: "This is a critical issue.",
		},
	}, nil
}

// setupTestParser creates a new in-memory DuckDB parser with the test validator.
func setupTestParser(t *testing.T, validator Validator) (*Parser, *sql.DB, func()) {
	t.Helper()
	db, err := sql.Open("duckdb", "")
	require.NoError(t, err)

	parser := New(db, validator)
	require.NoError(t, parser.Init())

	cleanup := func() {
		db.Close()
	}
	return parser, db, cleanup
}

// createTestExcel generates a test Excel file with specified VMs.
// Uses the exact column names expected by the ingestion template.
func createTestExcel(t *testing.T, vms []map[string]string, hosts []map[string]string) string {
	t.Helper()

	f := excelize.NewFile()
	defer f.Close()

	// Create vInfo sheet with exact column names from ingestion template
	vInfoIndex, err := f.NewSheet("vInfo")
	require.NoError(t, err)
	f.SetActiveSheet(vInfoIndex)

	// Column names must match the ingestion template exactly
	vInfoHeaders := []string{
		"VM", "VM ID", "VI SDK UUID", "Host", "CPUs", "Memory", "Powerstate",
		"Cluster", "Datacenter", "Template", "CBT", "Firmware", "Connection state",
		"FT State", "EnableUUID", "Folder", "DNS Name", "Primary IP Address",
		"In Use MiB", "HW version", "Provisioned MiB", "Resource pool",
		"OS according to the configuration file", "OS according to the VMware Tools",
		"VM UUID", "Total disk capacity MiB",
	}
	for i, h := range vInfoHeaders {
		cellRef := fmt.Sprintf("%s1", columnLetter(i))
		require.NoError(t, f.SetCellValue("vInfo", cellRef, h))
	}
	for rowIdx, vm := range vms {
		row := rowIdx + 2
		for colIdx, header := range vInfoHeaders {
			cellRef := fmt.Sprintf("%s%d", columnLetter(colIdx), row)
			if val, ok := vm[header]; ok {
				require.NoError(t, f.SetCellValue("vInfo", cellRef, val))
			}
		}
	}

	// Create vHost sheet
	_, err = f.NewSheet("vHost")
	require.NoError(t, err)
	vHostHeaders := []string{"Datacenter", "Cluster", "# Cores", "# CPU", "Object ID", "# Memory", "Model", "Vendor", "Host", "Config status"}
	for i, h := range vHostHeaders {
		cellRef := fmt.Sprintf("%s1", columnLetter(i))
		require.NoError(t, f.SetCellValue("vHost", cellRef, h))
	}
	for rowIdx, host := range hosts {
		row := rowIdx + 2
		for colIdx, header := range vHostHeaders {
			cellRef := fmt.Sprintf("%s%d", columnLetter(colIdx), row)
			if val, ok := host[header]; ok {
				require.NoError(t, f.SetCellValue("vHost", cellRef, val))
			}
		}
	}

	// Create vDatastore sheet
	_, err = f.NewSheet("vDatastore")
	require.NoError(t, err)
	dsHeaders := []string{"Hosts", "Address", "Name", "Object ID", "Free MiB", "MHA", "Capacity MiB", "Type"}
	for i, h := range dsHeaders {
		cellRef := fmt.Sprintf("%s1", columnLetter(i))
		require.NoError(t, f.SetCellValue("vDatastore", cellRef, h))
	}
	dsRow := []string{"esxi-host-1", "10.0.0.1", "datastore1", "datastore-001", "524288", "false", "1048576", "VMFS"}
	for i, val := range dsRow {
		cellRef := fmt.Sprintf("%s2", columnLetter(i))
		require.NoError(t, f.SetCellValue("vDatastore", cellRef, val))
	}

	// Create vCluster sheet for cluster ID resolution
	_, err = f.NewSheet("vCluster")
	require.NoError(t, err)
	vClusterHeaders := []string{"Name", "Object ID"}
	for i, h := range vClusterHeaders {
		cellRef := fmt.Sprintf("%s1", columnLetter(i))
		require.NoError(t, f.SetCellValue("vCluster", cellRef, h))
	}
	// Add cluster entries from hosts
	clustersSeen := make(map[string]bool)
	clusterRow := 2
	for _, host := range hosts {
		if cluster, ok := host["Cluster"]; ok && !clustersSeen[cluster] {
			clustersSeen[cluster] = true
			require.NoError(t, f.SetCellValue("vCluster", fmt.Sprintf("A%d", clusterRow), cluster))
			require.NoError(t, f.SetCellValue("vCluster", fmt.Sprintf("B%d", clusterRow), fmt.Sprintf("domain-c%d", clusterRow)))
			clusterRow++
		}
	}

	// Delete default Sheet1
	_ = f.DeleteSheet("Sheet1")

	// Write to temp file inside t.TempDir() so cleanup is automatic
	tmpPath := filepath.Join(t.TempDir(), "test-rvtools.xlsx")
	require.NoError(t, f.SaveAs(tmpPath))

	return tmpPath
}

// createTestExcelWithCustomHeaders generates a test Excel file with a custom vInfo header set.
// Used to test scenarios where required columns (e.g. "VM ID") are missing from the sheet.
func createTestExcelWithCustomHeaders(t *testing.T, vInfoHeaders []string, vms []map[string]string) string {
	t.Helper()

	f := excelize.NewFile()
	defer f.Close()

	vInfoIndex, err := f.NewSheet("vInfo")
	require.NoError(t, err)
	f.SetActiveSheet(vInfoIndex)

	for i, h := range vInfoHeaders {
		cellRef := fmt.Sprintf("%s1", columnLetter(i))
		require.NoError(t, f.SetCellValue("vInfo", cellRef, h))
	}
	for rowIdx, vm := range vms {
		row := rowIdx + 2
		for colIdx, header := range vInfoHeaders {
			cellRef := fmt.Sprintf("%s%d", columnLetter(colIdx), row)
			if val, ok := vm[header]; ok {
				require.NoError(t, f.SetCellValue("vInfo", cellRef, val))
			}
		}
	}

	_ = f.DeleteSheet("Sheet1")

	// Write to temp file inside t.TempDir() so cleanup is automatic
	tmpPath := filepath.Join(t.TempDir(), "test-rvtools.xlsx")
	require.NoError(t, f.SaveAs(tmpPath))

	return tmpPath
}

func columnLetter(col int) string {
	name, _ := excelize.ColumnNumberToName(col + 1)
	return name
}

func TestBuildInventory_BasicStructure(t *testing.T) {
	parser, _, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()

	vms := []map[string]string{
		{"VM": "vm-1", "VM ID": "vm-001", "VI SDK UUID": "uuid-1", "Host": "esxi-host-1", "CPUs": "4", "Memory": "8192", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"},
		{"VM": "vm-2", "VM ID": "vm-002", "VI SDK UUID": "uuid-2", "Host": "esxi-host-1", "CPUs": "2", "Memory": "4096", "Powerstate": "poweredOff", "Cluster": "cluster1", "Datacenter": "dc1"},
	}
	hosts := []map[string]string{
		{"Datacenter": "dc1", "Cluster": "cluster1", "# Cores": "8", "# CPU": "2", "Object ID": "host-001", "# Memory": "32768", "Model": "ESXi", "Vendor": "VMware", "Host": "esxi-host-1", "Config status": "green"},
	}

	tmpFile := createTestExcel(t, vms, hosts)

	ctx := context.Background()
	_, err := parser.IngestRvTools(ctx, tmpFile)
	require.NoError(t, err)

	inv, err := parser.BuildInventory(ctx)
	require.NoError(t, err)

	// Verify basic structure
	require.NotNil(t, inv)
	require.NotNil(t, inv.VCenter, "VCenter data should be populated")
	require.NotEmpty(t, inv.Clusters, "Clusters should be populated")
}

func TestBuildInventory_VMCounts(t *testing.T) {
	parser, _, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()

	vms := []map[string]string{
		{"VM": "vm-1", "VM ID": "vm-001", "VI SDK UUID": "uuid-1", "Host": "esxi-host-1", "CPUs": "4", "Memory": "8192", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"},
		{"VM": "vm-2", "VM ID": "vm-002", "VI SDK UUID": "uuid-2", "Host": "esxi-host-1", "CPUs": "2", "Memory": "4096", "Powerstate": "poweredOff", "Cluster": "cluster1", "Datacenter": "dc1"},
	}
	hosts := []map[string]string{
		{"Datacenter": "dc1", "Cluster": "cluster1", "# Cores": "8", "# CPU": "2", "Object ID": "host-001", "# Memory": "32768", "Model": "ESXi", "Vendor": "VMware", "Host": "esxi-host-1", "Config status": "green"},
	}

	tmpFile := createTestExcel(t, vms, hosts)

	ctx := context.Background()
	_, err := parser.IngestRvTools(ctx, tmpFile)
	require.NoError(t, err)

	inv, err := parser.BuildInventory(ctx)
	require.NoError(t, err)

	// VCenter total should equal sum of VMs
	assert.Equal(t, 2, inv.VCenter.VMs.Total)

	// Sum of cluster VMs should equal vCenter total
	var clusterTotal int
	for _, cluster := range inv.Clusters {
		clusterTotal += cluster.VMs.Total
	}
	assert.Equal(t, inv.VCenter.VMs.Total, clusterTotal)
}

func TestBuildInventory_PowerStates(t *testing.T) {
	parser, _, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()

	vms := []map[string]string{
		{"VM": "vm-1", "VM ID": "vm-001", "VI SDK UUID": "uuid-1", "Host": "esxi-host-1", "CPUs": "4", "Memory": "8192", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"},
		{"VM": "vm-2", "VM ID": "vm-002", "VI SDK UUID": "uuid-2", "Host": "esxi-host-1", "CPUs": "2", "Memory": "4096", "Powerstate": "poweredOff", "Cluster": "cluster1", "Datacenter": "dc1"},
		{"VM": "vm-3", "VM ID": "vm-003", "VI SDK UUID": "uuid-3", "Host": "esxi-host-1", "CPUs": "2", "Memory": "4096", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"},
	}
	hosts := []map[string]string{
		{"Datacenter": "dc1", "Cluster": "cluster1", "# Cores": "8", "# CPU": "2", "Object ID": "host-001", "# Memory": "32768", "Model": "ESXi", "Vendor": "VMware", "Host": "esxi-host-1", "Config status": "green"},
	}

	tmpFile := createTestExcel(t, vms, hosts)

	ctx := context.Background()
	_, err := parser.IngestRvTools(ctx, tmpFile)
	require.NoError(t, err)

	inv, err := parser.BuildInventory(ctx)
	require.NoError(t, err)

	// Verify power states
	assert.Equal(t, 2, inv.VCenter.VMs.PowerStates["poweredOn"])
	assert.Equal(t, 1, inv.VCenter.VMs.PowerStates["poweredOff"])
}

func TestBuildInventory_MigrationIssues(t *testing.T) {
	// Use warning validator to populate concerns
	parser, _, cleanup := setupTestParser(t, &testWarningValidator{})
	defer cleanup()

	vms := []map[string]string{
		{"VM": "vm-1", "VM ID": "vm-001", "VI SDK UUID": "uuid-1", "Host": "esxi-host-1", "CPUs": "4", "Memory": "8192", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"},
		{"VM": "vm-2", "VM ID": "vm-002", "VI SDK UUID": "uuid-2", "Host": "esxi-host-1", "CPUs": "2", "Memory": "4096", "Powerstate": "poweredOff", "Cluster": "cluster1", "Datacenter": "dc1"},
	}
	hosts := []map[string]string{
		{"Datacenter": "dc1", "Cluster": "cluster1", "# Cores": "8", "# CPU": "2", "Object ID": "host-001", "# Memory": "32768", "Model": "ESXi", "Vendor": "VMware", "Host": "esxi-host-1", "Config status": "green"},
	}

	tmpFile := createTestExcel(t, vms, hosts)

	ctx := context.Background()
	_, err := parser.IngestRvTools(ctx, tmpFile)
	require.NoError(t, err)

	inv, err := parser.BuildInventory(ctx)
	require.NoError(t, err)

	// Verify migration warnings are populated
	require.NotEmpty(t, inv.VCenter.VMs.MigrationWarnings, "Migration warnings should be populated")
	assert.Equal(t, "test.warning", inv.VCenter.VMs.MigrationWarnings[0].ID)
	assert.Equal(t, 2, inv.VCenter.VMs.MigrationWarnings[0].Count, "Warning count should match VM count")
}

func TestBuildInventory_ResourceBreakdowns(t *testing.T) {
	parser, _, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()

	vms := []map[string]string{
		{"VM": "vm-1", "VM ID": "vm-001", "VI SDK UUID": "uuid-1", "Host": "esxi-host-1", "CPUs": "4", "Memory": "8192", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"},
		{"VM": "vm-2", "VM ID": "vm-002", "VI SDK UUID": "uuid-2", "Host": "esxi-host-1", "CPUs": "2", "Memory": "4096", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"},
	}
	hosts := []map[string]string{
		{"Datacenter": "dc1", "Cluster": "cluster1", "# Cores": "8", "# CPU": "2", "Object ID": "host-001", "# Memory": "32768", "Model": "ESXi", "Vendor": "VMware", "Host": "esxi-host-1", "Config status": "green"},
	}

	tmpFile := createTestExcel(t, vms, hosts)

	ctx := context.Background()
	_, err := parser.IngestRvTools(ctx, tmpFile)
	require.NoError(t, err)

	inv, err := parser.BuildInventory(ctx)
	require.NoError(t, err)

	// Verify CPU cores total (4 + 2 = 6)
	assert.Equal(t, 6, inv.VCenter.VMs.CPUCores.Total)

	// Verify RAM total (8192 + 4096 = 12288 MB = 12 GB)
	assert.Equal(t, 12, inv.VCenter.VMs.RamGB.Total)
}

func TestBuildInventory_InfraData(t *testing.T) {
	parser, _, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()

	vms := []map[string]string{
		{"VM": "vm-1", "VM ID": "vm-001", "VI SDK UUID": "uuid-1", "Host": "esxi-host-1", "CPUs": "4", "Memory": "8192", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"},
	}
	hosts := []map[string]string{
		{"Datacenter": "dc1", "Cluster": "cluster1", "# Cores": "8", "# CPU": "2", "Object ID": "host-001", "# Memory": "32768", "Model": "PowerEdge", "Vendor": "Dell", "Host": "esxi-host-1", "Config status": "green"},
		{"Datacenter": "dc1", "Cluster": "cluster1", "# Cores": "16", "# CPU": "2", "Object ID": "host-002", "# Memory": "65536", "Model": "ProLiant", "Vendor": "HP", "Host": "esxi-host-2", "Config status": "green"},
	}

	tmpFile := createTestExcel(t, vms, hosts)

	ctx := context.Background()
	_, err := parser.IngestRvTools(ctx, tmpFile)
	require.NoError(t, err)

	inv, err := parser.BuildInventory(ctx)
	require.NoError(t, err)

	// Verify hosts are populated
	assert.Equal(t, 2, inv.VCenter.Infra.TotalHosts)
	assert.Len(t, inv.VCenter.Infra.Hosts, 2)

	// Verify datastores are populated
	assert.NotEmpty(t, inv.VCenter.Infra.Datastores)
}

func TestBuildInventory_ClusterInventories(t *testing.T) {
	parser, _, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()

	vms := []map[string]string{
		{"VM": "vm-1", "VM ID": "vm-001", "VI SDK UUID": "uuid-1", "Host": "esxi-host-1", "CPUs": "4", "Memory": "8192", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"},
	}
	hosts := []map[string]string{
		{"Datacenter": "dc1", "Cluster": "cluster1", "# Cores": "8", "# CPU": "2", "Object ID": "host-001", "# Memory": "32768", "Model": "ESXi", "Vendor": "VMware", "Host": "esxi-host-1", "Config status": "green"},
	}

	tmpFile := createTestExcel(t, vms, hosts)

	ctx := context.Background()
	_, err := parser.IngestRvTools(ctx, tmpFile)
	require.NoError(t, err)

	inv, err := parser.BuildInventory(ctx)
	require.NoError(t, err)

	// Per-cluster inventories should have correct structure
	require.Len(t, inv.Clusters, 1)

	for _, clusterData := range inv.Clusters {
		// Per-cluster should have 1 datacenter and 1 cluster
		assert.Equal(t, 1, clusterData.Infra.TotalDatacenters)
		assert.Equal(t, []int{1}, clusterData.Infra.ClustersPerDatacenter)
	}
}

func TestBuildInventory_EmptyData(t *testing.T) {
	parser, _, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()

	// Empty vms and hosts
	vms := []map[string]string{}
	hosts := []map[string]string{}

	tmpFile := createTestExcel(t, vms, hosts)

	ctx := context.Background()
	result, err := parser.IngestRvTools(ctx, tmpFile)
	require.NoError(t, err)

	// Schema should have errors due to empty vInfo
	if result.HasErrors() {
		// Expected - empty data fails validation
		return
	}

	// If no errors, inventory should still be valid but empty
	inv, err := parser.BuildInventory(ctx)
	require.NoError(t, err)
	require.NotNil(t, inv)
	assert.Equal(t, 0, inv.VCenter.VMs.Total)
}

func TestValidation_ErrorCodes(t *testing.T) {
	defaultHosts := []map[string]string{
		{"Datacenter": "dc1", "Cluster": "cluster1", "# Cores": "8", "# CPU": "2", "Object ID": "host-001", "# Memory": "32768", "Model": "ESXi", "Vendor": "VMware", "Host": "esxi-host-1", "Config status": "green"},
	}

	tests := []struct {
		name           string
		vms            []map[string]string
		hosts          []map[string]string
		customHeaders  []string // if set, uses createTestExcelWithCustomHeaders
		expectedCodes  []string // codes that must be present
		forbiddenCodes []string // codes that must NOT be present
	}{
		{
			name:           "empty vInfo with missing VM ID column reports NO_VMS only",
			customHeaders:  []string{"VM", "Host", "CPUs", "Memory", "Powerstate", "Cluster", "Datacenter"},
			vms:            []map[string]string{},
			expectedCodes:  []string{CodeNoVMs},
			forbiddenCodes: []string{CodeMissingVMID, CodeMissingVMName},
		},
		{
			name:           "rows without VM IDs report MISSING_VM_ID",
			vms:            []map[string]string{{"VM": "vm-1", "Host": "esxi-host-1", "CPUs": "4", "Memory": "8192", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"}},
			hosts:          defaultHosts,
			expectedCodes:  []string{CodeMissingVMID},
			forbiddenCodes: []string{CodeNoVMs},
		},
		{
			name:           "rows without VM names report MISSING_VM_NAME",
			vms:            []map[string]string{{"VM ID": "vm-001", "Host": "esxi-host-1", "CPUs": "4", "Memory": "8192", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"}},
			hosts:          defaultHosts,
			expectedCodes:  []string{CodeMissingVMName},
			forbiddenCodes: []string{CodeNoVMs},
		},
		{
			name:           "missing VM ID column and empty VM values reports both errors",
			customHeaders:  []string{"VM", "Host", "CPUs", "Memory", "Powerstate", "Cluster", "Datacenter"},
			vms:            []map[string]string{{"VM": "", "Host": "esxi-host-1", "CPUs": "4", "Memory": "8192", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"}},
			expectedCodes:  []string{CodeMissingVMID, CodeMissingVMName},
			forbiddenCodes: []string{CodeNoVMs},
		},
		{
			name:           "missing Cluster column reports MISSING_CLUSTER",
			customHeaders:  []string{"VM", "VM ID", "Host", "CPUs", "Memory", "Powerstate", "Datacenter"},
			vms:            []map[string]string{{"VM": "vm-1", "VM ID": "vm-001", "Host": "esxi-host-1", "CPUs": "4", "Memory": "8192", "Powerstate": "poweredOn", "Datacenter": "dc1"}},
			expectedCodes:  []string{CodeMissingCluster},
			forbiddenCodes: []string{CodeNoVMs, CodeMissingVMID, CodeMissingVMName},
		},
		{
			name:           "empty Cluster values reports MISSING_CLUSTER",
			customHeaders:  []string{"VM", "VM ID", "Host", "CPUs", "Memory", "Powerstate", "Cluster", "Datacenter"},
			vms:            []map[string]string{{"VM": "vm-1", "VM ID": "vm-001", "Host": "esxi-host-1", "CPUs": "4", "Memory": "8192", "Powerstate": "poweredOn", "Cluster": "", "Datacenter": "dc1"}},
			expectedCodes:  []string{CodeMissingCluster},
			forbiddenCodes: []string{CodeNoVMs, CodeMissingVMID, CodeMissingVMName},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, _, cleanup := setupTestParser(t, &testValidator{})
			defer cleanup()

			var tmpFile string
			if tt.customHeaders != nil {
				tmpFile = createTestExcelWithCustomHeaders(t, tt.customHeaders, tt.vms)
			} else {
				tmpFile = createTestExcel(t, tt.vms, tt.hosts)
			}

			result, err := parser.IngestRvTools(context.Background(), tmpFile)
			require.NoError(t, err)
			require.True(t, result.HasErrors())

			errorCodes := make(map[string]bool)
			for _, e := range result.Errors {
				errorCodes[e.Code] = true
			}
			for _, code := range tt.expectedCodes {
				assert.True(t, errorCodes[code], "expected error code %s", code)
			}
			for _, code := range tt.forbiddenCodes {
				assert.False(t, errorCodes[code], "unexpected error code %s", code)
			}
		})
	}
}

func TestBuildInventory_MultiCluster(t *testing.T) {
	parser, _, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()

	vms := []map[string]string{
		{"VM": "vm-1", "VM ID": "vm-001", "VI SDK UUID": "uuid-1", "Host": "esxi-host-1", "CPUs": "4", "Memory": "8192", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"},
		{"VM": "vm-2", "VM ID": "vm-002", "VI SDK UUID": "uuid-2", "Host": "esxi-host-1", "CPUs": "2", "Memory": "4096", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"},
		{"VM": "vm-3", "VM ID": "vm-003", "VI SDK UUID": "uuid-3", "Host": "esxi-host-2", "CPUs": "4", "Memory": "8192", "Powerstate": "poweredOn", "Cluster": "cluster2", "Datacenter": "dc1"},
		{"VM": "vm-4", "VM ID": "vm-004", "VI SDK UUID": "uuid-4", "Host": "esxi-host-2", "CPUs": "8", "Memory": "16384", "Powerstate": "poweredOff", "Cluster": "cluster2", "Datacenter": "dc1"},
	}
	hosts := []map[string]string{
		{"Datacenter": "dc1", "Cluster": "cluster1", "# Cores": "8", "# CPU": "2", "Object ID": "host-001", "# Memory": "32768", "Model": "ESXi", "Vendor": "VMware", "Host": "esxi-host-1", "Config status": "green"},
		{"Datacenter": "dc1", "Cluster": "cluster2", "# Cores": "16", "# CPU": "2", "Object ID": "host-002", "# Memory": "65536", "Model": "ESXi", "Vendor": "VMware", "Host": "esxi-host-2", "Config status": "green"},
	}

	tmpFile := createTestExcel(t, vms, hosts)

	ctx := context.Background()
	_, err := parser.IngestRvTools(ctx, tmpFile)
	require.NoError(t, err)

	inv, err := parser.BuildInventory(ctx)
	require.NoError(t, err)

	// VCenter should have all 4 VMs
	assert.Equal(t, 4, inv.VCenter.VMs.Total)

	// Should have 2 clusters
	assert.Len(t, inv.Clusters, 2)

	// Sum of cluster VMs should equal vCenter total
	var clusterTotal int
	for _, cluster := range inv.Clusters {
		clusterTotal += cluster.VMs.Total
	}
	assert.Equal(t, 4, clusterTotal)
}

func TestResolveClusterID(t *testing.T) {
	tests := []struct {
		name           string
		clusterName    string
		objectIDs      map[string]string
		datacenters    map[string]string
		vcenterID      string
		expectObjectID bool // if true, expect the objectID value
		expectedID     string
	}{
		{
			name:           "uses Object ID from vCluster when available",
			clusterName:    "cluster1",
			objectIDs:      map[string]string{"cluster1": "domain-c123"},
			datacenters:    map[string]string{"cluster1": "dc1"},
			vcenterID:      "vcenter-uuid",
			expectObjectID: true,
			expectedID:     "domain-c123",
		},
		{
			name:           "falls back to generated ID when missing",
			clusterName:    "cluster2",
			objectIDs:      map[string]string{"cluster1": "domain-c123"},
			datacenters:    map[string]string{"cluster2": "dc1"},
			vcenterID:      "vcenter-uuid",
			expectObjectID: false,
		},
		{
			name:           "handles empty maps",
			clusterName:    "cluster1",
			objectIDs:      map[string]string{},
			datacenters:    map[string]string{},
			vcenterID:      "vcenter-uuid",
			expectObjectID: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveClusterID(tt.clusterName, tt.objectIDs, tt.datacenters, tt.vcenterID)

			if tt.expectObjectID {
				assert.Equal(t, tt.expectedID, result)
			} else {
				// Should be a generated ID in format cluster-{16hexchars}
				assert.True(t, strings.HasPrefix(result, "cluster-"), "Generated ID should start with 'cluster-'")
				assert.Len(t, result, len("cluster-")+16, "Generated ID should have correct length")
			}
		})
	}
}

func TestGenerateClusterID(t *testing.T) {
	tests := []struct {
		name           string
		clusterName    string
		datacenterName string
		vcenterID      string
	}{
		{
			name:           "basic generation",
			clusterName:    "cluster1",
			datacenterName: "dc1",
			vcenterID:      "vcenter-uuid",
		},
		{
			name:           "different cluster",
			clusterName:    "cluster2",
			datacenterName: "dc1",
			vcenterID:      "vcenter-uuid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := generateClusterID(tt.clusterName, tt.datacenterName, tt.vcenterID)

			// Verify format: cluster-{16hexchars}
			assert.True(t, strings.HasPrefix(id, "cluster-"))
			assert.Len(t, id, len("cluster-")+16)

			// Verify determinism - same inputs produce same output
			id2 := generateClusterID(tt.clusterName, tt.datacenterName, tt.vcenterID)
			assert.Equal(t, id, id2, "Same inputs should produce same output")
		})
	}

	// Test that different inputs produce different outputs
	id1 := generateClusterID("cluster1", "dc1", "vcenter-1")
	id2 := generateClusterID("cluster2", "dc1", "vcenter-1")
	id3 := generateClusterID("cluster1", "dc2", "vcenter-1")
	id4 := generateClusterID("cluster1", "dc1", "vcenter-2")

	assert.NotEqual(t, id1, id2, "Different cluster names should produce different IDs")
	assert.NotEqual(t, id1, id3, "Different datacenter names should produce different IDs")
	assert.NotEqual(t, id1, id4, "Different vCenter IDs should produce different IDs")
}

func TestBuildInventory_Overcommitment(t *testing.T) {
	parser, _, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()

	// VMs that consume resources
	vms := []map[string]string{
		{"VM": "vm-1", "VM ID": "vm-001", "VI SDK UUID": "uuid-1", "Host": "esxi-host-1", "CPUs": "8", "Memory": "16384", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"},
		{"VM": "vm-2", "VM ID": "vm-002", "VI SDK UUID": "uuid-2", "Host": "esxi-host-1", "CPUs": "8", "Memory": "16384", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"},
	}
	// Host with 8 cores and 32GB memory
	hosts := []map[string]string{
		{"Datacenter": "dc1", "Cluster": "cluster1", "# Cores": "8", "# CPU": "2", "Object ID": "host-001", "# Memory": "32768", "Model": "ESXi", "Vendor": "VMware", "Host": "esxi-host-1", "Config status": "green"},
	}

	tmpFile := createTestExcel(t, vms, hosts)

	ctx := context.Background()
	_, err := parser.IngestRvTools(ctx, tmpFile)
	require.NoError(t, err)

	inv, err := parser.BuildInventory(ctx)
	require.NoError(t, err)

	// With hosts, overcommitment should be calculated
	// 2 VMs with 8 vCPUs each = 16 vCPUs, host has 8 cores = 2.0 overcommit
	require.NotNil(t, inv.VCenter.Infra.CPUOverCommitment)
	assert.Equal(t, 2.0, *inv.VCenter.Infra.CPUOverCommitment)

	// Memory: 2 VMs with 16GB each = 32GB, host has 32GB = 1.0 overcommit
	require.NotNil(t, inv.VCenter.Infra.MemoryOverCommitment)
	assert.Equal(t, 1.0, *inv.VCenter.Infra.MemoryOverCommitment)
}

func TestBuildInventory_MigratableCounts(t *testing.T) {
	// Use critical validator to make VMs not migratable
	parser, _, cleanup := setupTestParser(t, &testCriticalValidator{})
	defer cleanup()

	vms := []map[string]string{
		{"VM": "vm-1", "VM ID": "vm-001", "VI SDK UUID": "uuid-1", "Host": "esxi-host-1", "CPUs": "4", "Memory": "8192", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"},
		{"VM": "vm-2", "VM ID": "vm-002", "VI SDK UUID": "uuid-2", "Host": "esxi-host-1", "CPUs": "2", "Memory": "4096", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"},
	}
	hosts := []map[string]string{
		{"Datacenter": "dc1", "Cluster": "cluster1", "# Cores": "8", "# CPU": "2", "Object ID": "host-001", "# Memory": "32768", "Model": "ESXi", "Vendor": "VMware", "Host": "esxi-host-1", "Config status": "green"},
	}

	tmpFile := createTestExcel(t, vms, hosts)

	ctx := context.Background()
	_, err := parser.IngestRvTools(ctx, tmpFile)
	require.NoError(t, err)

	inv, err := parser.BuildInventory(ctx)
	require.NoError(t, err)

	// With critical concerns, VMs should not be migratable
	assert.Equal(t, 2, inv.VCenter.VMs.Total)
	assert.Equal(t, 0, inv.VCenter.VMs.TotalMigratable, "VMs with critical concerns should not be migratable")

	// Critical concerns should be in NotMigratableReasons
	require.NotEmpty(t, inv.VCenter.VMs.NotMigratableReasons)
	assert.Equal(t, "test.critical", inv.VCenter.VMs.NotMigratableReasons[0].ID)
}

func TestBuildInventory_MigratableWithWarnings(t *testing.T) {
	parser, _, cleanup := setupTestParser(t, &testWarningValidator{})
	defer cleanup()

	vms := []map[string]string{
		{"VM": "vm-1", "VM ID": "vm-001", "VI SDK UUID": "uuid-1", "Host": "esxi-host-1", "CPUs": "4", "Memory": "8192", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"},
		{"VM": "vm-2", "VM ID": "vm-002", "VI SDK UUID": "uuid-2", "Host": "esxi-host-1", "CPUs": "2", "Memory": "4096", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"},
	}
	hosts := []map[string]string{
		{"Datacenter": "dc1", "Cluster": "cluster1", "# Cores": "8", "# CPU": "2", "Object ID": "host-001", "# Memory": "32768", "Model": "ESXi", "Vendor": "VMware", "Host": "esxi-host-1", "Config status": "green"},
	}

	tmpFile := createTestExcel(t, vms, hosts)

	ctx := context.Background()
	_, err := parser.IngestRvTools(ctx, tmpFile)
	require.NoError(t, err)

	inv, err := parser.BuildInventory(ctx)
	require.NoError(t, err)

	// With warning concerns (no critical), VMs should be migratable but with warnings
	assert.Equal(t, 2, inv.VCenter.VMs.Total)
	assert.Equal(t, 2, inv.VCenter.VMs.TotalMigratable, "VMs with only warnings should be migratable")
	assert.Equal(t, 2, inv.VCenter.VMs.TotalMigratableWithWarnings, "VMs with warnings should be counted")
}

// TestBuildInventory_MinimalSchema tests that a file with only VM ID, VM, and Cluster
// columns (the minimal required schema) correctly ingests VMs and builds an inventory.
func TestBuildInventory_MinimalSchema(t *testing.T) {
	parser, _, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()

	// Create Excel with only the minimal required columns
	minimalHeaders := []string{"VM ID", "VM", "Cluster"}
	vms := []map[string]string{
		{"VM ID": "vm-100", "VM": "VM-SRV-0000", "Cluster": "cluster-1"},
		{"VM ID": "vm-101", "VM": "VM-SRV-0001", "Cluster": "cluster-1"},
		{"VM ID": "vm-102", "VM": "VM-SRV-0002", "Cluster": "cluster-1"},
	}

	tmpFile := createTestExcelWithCustomHeaders(t, minimalHeaders, vms)

	ctx := context.Background()
	result, err := parser.IngestRvTools(ctx, tmpFile)
	require.NoError(t, err)
	require.True(t, result.IsValid(), "Minimal required schema should pass validation: %v", result.Errors)

	inv, err := parser.BuildInventory(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, inv.VCenter.VMs.Total, "Should have 3 VMs from minimal schema")
}
