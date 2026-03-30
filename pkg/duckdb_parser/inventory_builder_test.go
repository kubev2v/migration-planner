package duckdb_parser

import (
	"context"
	"database/sql"
	"fmt"
	"os"
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
		_ = db.Close()
	}
	return parser, db, cleanup
}

// ExcelSheet defines a sheet (name, headers, rows) for createTestExcel.
// Row maps are keyed by header name; only provided keys are written.
type ExcelSheet struct {
	Name    string
	Headers []string
	Rows    []map[string]string
}

// NewExcelSheet builds a sheet value for createTestExcel.
func NewExcelSheet(name string, headers []string, rows []map[string]string) ExcelSheet {
	return ExcelSheet{Name: name, Headers: headers, Rows: rows}
}

// Standard sheet header sets for ingestion template compatibility.
var (
	vInfoHeaders = []string{
		"VM", "VM ID", "VI SDK UUID", "Host", "CPUs", "Memory", "Powerstate",
		"Cluster", "Datacenter", "Template", "CBT", "Firmware", "Connection state",
		"FT State", "EnableUUID", "Folder", "DNS Name", "Primary IP Address",
		"In Use MiB", "HW version", "Provisioned MiB", "Resource pool",
		"OS according to the configuration file", "OS according to the VMware Tools",
		"VM UUID", "Total disk capacity MiB",
	}
	vHostHeaders      = []string{"Datacenter", "Cluster", "# Cores", "# CPU", "Object ID", "# Memory", "Model", "Vendor", "Host", "Config status"}
	vDatastoreHeaders = []string{"Hosts", "Address", "Name", "Object ID", "Free MiB", "MHA", "Capacity MiB", "Type"}
	vClusterHeaders   = []string{"Name", "Object ID"}
)

// defaultStandardSheets returns vInfo, vHost, default vDatastore, and vCluster from hosts for createTestExcel.
func defaultStandardSheets(vms, hosts []map[string]string) []ExcelSheet {
	clustersSeen := make(map[string]bool)
	var vClustersRows []map[string]string
	for i, host := range hosts {
		cluster, ok := host["Cluster"]
		if !ok || clustersSeen[cluster] {
			continue
		}
		clustersSeen[cluster] = true
		vClustersRows = append(vClustersRows, map[string]string{"Name": cluster, "Object ID": fmt.Sprintf("domain-c%d", i+1)})
	}

	vDatastoreRows := []map[string]string{
		{
			"Hosts":        "esxi-host-1",
			"Address":      "10.0.0.1",
			"Name":         "datastore1",
			"Object ID":    "datastore-001",
			"Free MiB":     "524288",
			"MHA":          "false",
			"Capacity MiB": "1048576",
			"Type":         "VMFS",
		},
	}

	return []ExcelSheet{
		NewExcelSheet("vInfo", vInfoHeaders, vms),
		NewExcelSheet("vHost", vHostHeaders, hosts),
		NewExcelSheet("vDatastore", vDatastoreHeaders, vDatastoreRows),
		NewExcelSheet("vCluster", vClusterHeaders, vClustersRows),
	}
}

// createTestExcel generates a test Excel file from variadic sheets (vInfo, vHost, vDisk, etc.).
// Nothing is mandatory; pass only the sheets you need. Use NewExcelSheet(name, headers, rows).
func createTestExcel(t *testing.T, sheets ...ExcelSheet) string {
	t.Helper()

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	firstSheet := true
	for _, sh := range sheets {
		idx, err := f.NewSheet(sh.Name)
		require.NoError(t, err)
		if firstSheet {
			f.SetActiveSheet(idx)
			firstSheet = false
		}
		for i, h := range sh.Headers {
			cellRef := fmt.Sprintf("%s1", columnLetter(i))
			require.NoError(t, f.SetCellValue(sh.Name, cellRef, h))
		}
		for rowIdx, row := range sh.Rows {
			r := rowIdx + 2
			for colIdx, header := range sh.Headers {
				cellRef := fmt.Sprintf("%s%d", columnLetter(colIdx), r)
				if val, ok := row[header]; ok {
					require.NoError(t, f.SetCellValue(sh.Name, cellRef, val))
				}
			}
		}
	}

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
	defer func() { _ = f.Close() }()

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

	tmpFile := createTestExcel(t, defaultStandardSheets(vms, hosts)...)

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

	tmpFile := createTestExcel(t, defaultStandardSheets(vms, hosts)...)

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

	tmpFile := createTestExcel(t, defaultStandardSheets(vms, hosts)...)

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

	tmpFile := createTestExcel(t, defaultStandardSheets(vms, hosts)...)

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

	tmpFile := createTestExcel(t, defaultStandardSheets(vms, hosts)...)

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

	tmpFile := createTestExcel(t, defaultStandardSheets(vms, hosts)...)

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

	tmpFile := createTestExcel(t, defaultStandardSheets(vms, hosts)...)

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

	tmpFile := createTestExcel(t, defaultStandardSheets(vms, hosts)...)

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
			name:           "missing VM ID column and empty VM values reports query error and missing name",
			customHeaders:  []string{"VM", "VI SDK UUID", "Host", "CPUs", "Memory", "Powerstate", "Cluster", "Datacenter"},
			vms:            []map[string]string{{"VM": "", "VI SDK UUID": "550e8400-e29b-41d4-a716-446655440000", "Host": "esxi-host-1", "CPUs": "4", "Memory": "8192", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"}},
			expectedCodes:  []string{CodeColumnValidationFailed, CodeMissingVMName},
			forbiddenCodes: []string{CodeNoVMs},
		},
		{
			name:           "missing Cluster column reports MISSING_CLUSTER",
			customHeaders:  []string{"VM", "VM ID", "VI SDK UUID", "Host", "CPUs", "Memory", "Powerstate", "Datacenter"},
			vms:            []map[string]string{{"VM": "vm-1", "VM ID": "vm-001", "VI SDK UUID": "550e8400-e29b-41d4-a716-446655440000", "Host": "esxi-host-1", "CPUs": "4", "Memory": "8192", "Powerstate": "poweredOn", "Datacenter": "dc1"}},
			expectedCodes:  []string{CodeMissingCluster},
			forbiddenCodes: []string{CodeNoVMs, CodeMissingVMID, CodeMissingVMName},
		},
		{
			name:           "empty Cluster values reports MISSING_CLUSTER",
			customHeaders:  []string{"VM", "VM ID", "VI SDK UUID", "Host", "CPUs", "Memory", "Powerstate", "Cluster", "Datacenter"},
			vms:            []map[string]string{{"VM": "vm-1", "VM ID": "vm-001", "VI SDK UUID": "550e8400-e29b-41d4-a716-446655440000", "Host": "esxi-host-1", "CPUs": "4", "Memory": "8192", "Powerstate": "poweredOn", "Cluster": "", "Datacenter": "dc1"}},
			expectedCodes:  []string{CodeMissingCluster},
			forbiddenCodes: []string{CodeNoVMs, CodeMissingVMID, CodeMissingVMName},
		},
		{
			name:           "missing VI SDK UUID column reports MISSING_VI_SDK_UUID",
			customHeaders:  []string{"VM", "VM ID", "Host", "CPUs", "Memory", "Powerstate", "Cluster", "Datacenter"},
			vms:            []map[string]string{{"VM": "vm-1", "VM ID": "vm-001", "Host": "esxi-host-1", "CPUs": "4", "Memory": "8192", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"}},
			expectedCodes:  []string{CodeMissingVISDKUUID},
			forbiddenCodes: []string{CodeNoVMs},
		},
		{
			name:           "empty VI SDK UUID values report MISSING_VI_SDK_UUID",
			customHeaders:  []string{"VM", "VM ID", "VI SDK UUID", "Host", "CPUs", "Memory", "Powerstate", "Cluster", "Datacenter"},
			vms:            []map[string]string{{"VM": "vm-1", "VM ID": "vm-001", "VI SDK UUID": "", "Host": "esxi-host-1", "CPUs": "4", "Memory": "8192", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"}},
			expectedCodes:  []string{CodeMissingVISDKUUID},
			forbiddenCodes: []string{CodeNoVMs},
		},
		{
			name:           "whitespace-only VI SDK UUID values report MISSING_VI_SDK_UUID",
			customHeaders:  []string{"VM", "VM ID", "VI SDK UUID", "Host", "CPUs", "Memory", "Powerstate", "Cluster", "Datacenter"},
			vms:            []map[string]string{{"VM": "vm-1", "VM ID": "vm-001", "VI SDK UUID": "   ", "Host": "esxi-host-1", "CPUs": "4", "Memory": "8192", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"}},
			expectedCodes:  []string{CodeMissingVISDKUUID},
			forbiddenCodes: []string{CodeNoVMs},
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
				tmpFile = createTestExcel(t, defaultStandardSheets(tt.vms, tt.hosts)...)
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

	tmpFile := createTestExcel(t, defaultStandardSheets(vms, hosts)...)

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

	tmpFile := createTestExcel(t, defaultStandardSheets(vms, hosts)...)

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

	tmpFile := createTestExcel(t, defaultStandardSheets(vms, hosts)...)

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

	tmpFile := createTestExcel(t, defaultStandardSheets(vms, hosts)...)

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

// TestBuildInventory_MinimalSchema tests that a file with only VM ID, VM, Cluster, and VI SDK UUID
// columns (the minimal required schema for RVTools) correctly ingests VMs and builds an inventory.
func TestBuildInventory_MinimalSchema(t *testing.T) {
	parser, _, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()

	vcUUID := "550e8400-e29b-41d4-a716-446655440000"
	minimalHeaders := []string{"VM ID", "VM", "Cluster", "VI SDK UUID"}
	vms := []map[string]string{
		{"VM ID": "vm-100", "VM": "VM-SRV-0000", "Cluster": "cluster-1", "VI SDK UUID": vcUUID},
		{"VM ID": "vm-101", "VM": "VM-SRV-0001", "Cluster": "cluster-1", "VI SDK UUID": vcUUID},
		{"VM ID": "vm-102", "VM": "VM-SRV-0002", "Cluster": "cluster-1", "VI SDK UUID": vcUUID},
	}

	tmpFile := createTestExcelWithCustomHeaders(t, minimalHeaders, vms)

	ctx := context.Background()
	result, err := parser.IngestRvTools(ctx, tmpFile)
	require.NoError(t, err)
	require.True(t, result.IsValid(), "Minimal required schema should pass validation: %v", result.Errors)

	inv, err := parser.BuildInventory(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, inv.VCenter.VMs.Total, "Should have 3 VMs from minimal schema")
	assert.Equal(t, vcUUID, inv.VCenterID, "VCenterID should be populated from VI SDK UUID column")
}

// TestBuildInventory_VMsWithSharedDisksCount ingests Excel with vDisk data and asserts
// VMsWithSharedDisksCount returns the count of VMs that have at least one shared disk.
func TestBuildInventory_VMsWithSharedDisksCount(t *testing.T) {
	parser, _, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()

	vms := []map[string]string{
		{"VM": "vm-1", "VM ID": "vm-001", "VI SDK UUID": "uuid-1", "Host": "esxi-host-1", "CPUs": "4", "Memory": "8192", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"},
		{"VM": "vm-2", "VM ID": "vm-002", "VI SDK UUID": "uuid-2", "Host": "esxi-host-1", "CPUs": "2", "Memory": "4096", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"},
		{"VM": "vm-3", "VM ID": "vm-003", "VI SDK UUID": "uuid-3", "Host": "esxi-host-1", "CPUs": "2", "Memory": "4096", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"},
		{"VM": "vm-4", "VM ID": "vm-004", "VI SDK UUID": "uuid-4", "Host": "esxi-host-1", "CPUs": "2", "Memory": "4096", "Powerstate": "poweredOn", "Cluster": "cluster2", "Datacenter": "dc1"},
	}
	hosts := []map[string]string{
		{"Datacenter": "dc1", "Cluster": "cluster1", "# Cores": "8", "# CPU": "2", "Object ID": "host-001", "# Memory": "32768", "Model": "ESXi", "Vendor": "VMware", "Host": "esxi-host-1", "Config status": "green"},
		{"Datacenter": "dc1", "Cluster": "cluster2", "# Cores": "8", "# CPU": "2", "Object ID": "host-002", "# Memory": "32768", "Model": "ESXi", "Vendor": "VMware", "Host": "esxi-host-2", "Config status": "green"},
	}
	// vm-001: one shared disk -> counted
	// vm-002: only non-shared disks -> not counted
	// vm-003: one shared, one non-shared -> counted
	// vm-004: one shared (cluster2) -> counted; filter by cluster1 should exclude it
	vDiskHeaders := []string{
		"VM ID", "Disk Key", "Unit #", "Path", "Disk Path", "Capacity MiB",
		"Sharing mode", "Raw", "Shared Bus", "Disk Mode", "Disk UUID",
		"Thin", "Controller", "Label", "SCSI Unit #",
	}
	disks := []map[string]string{
		{"VM ID": "vm-001", "Disk Key": "2000", "Unit #": "0", "Path": "[ds1] vm-1/disk.vmdk", "Capacity MiB": "10240", "Sharing mode": "true"},
		{"VM ID": "vm-002", "Disk Key": "2001", "Unit #": "0", "Path": "[ds1] vm-2/disk.vmdk", "Capacity MiB": "8192", "Sharing mode": "false"},
		{"VM ID": "vm-003", "Disk Key": "2002", "Unit #": "0", "Path": "[ds1] vm-3/disk0.vmdk", "Capacity MiB": "4096", "Sharing mode": "true"},
		{"VM ID": "vm-003", "Disk Key": "2003", "Unit #": "1", "Path": "[ds1] vm-3/disk1.vmdk", "Capacity MiB": "2048", "Sharing mode": "false"},
		{"VM ID": "vm-004", "Disk Key": "2004", "Unit #": "0", "Path": "[ds1] vm-4/disk.vmdk", "Capacity MiB": "4096", "Sharing mode": "true"},
	}

	tmpFile := createTestExcel(t, append(defaultStandardSheets(vms, hosts), NewExcelSheet("vDisk", vDiskHeaders, disks))...)
	defer func() { _ = os.Remove(tmpFile) }()

	ctx := context.Background()
	_, err := parser.IngestRvTools(ctx, tmpFile)
	require.NoError(t, err)

	// No filter: 3 VMs with at least one shared disk (vm-001, vm-003, vm-004)
	count, err := parser.VMsWithSharedDisksCount(ctx, Filters{})
	require.NoError(t, err)
	assert.Equal(t, 3, count, "VMs with at least one shared disk (vm-001, vm-003, vm-004)")

	// Filter by cluster1: 2 VMs (vm-001, vm-003); vm-004 is in cluster2
	countCluster1, err := parser.VMsWithSharedDisksCount(ctx, Filters{Cluster: "cluster1"})
	require.NoError(t, err)
	assert.Equal(t, 2, countCluster1, "VMs with shared disks in cluster1 only")

	countCluster2, err := parser.VMsWithSharedDisksCount(ctx, Filters{Cluster: "cluster2"})
	require.NoError(t, err)
	assert.Equal(t, 1, countCluster2, "VMs with shared disks in cluster2 only")
}

func TestBuildInventory_ComplexityDistributionWithDiskSize(t *testing.T) {
	parser, _, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()

	// Three VMs:
	//   vm-1 (cluster-a): Easy (complexity=1), disk 2048 MiB ≈ 0.002 TB
	//   vm-2 (cluster-a): Medium (complexity=2), disk 10240 MiB ≈ 0.01 TB
	//   vm-3 (cluster-b): Unknown (complexity=0, DB default), disk 5120 MiB ≈ 0.005 TB
	vms := []map[string]string{
		{
			"VM": "vm-easy", "VM ID": "vm-1", "CPUs": "2", "Memory": "4096",
			"Powerstate": "poweredOn", "Cluster": "cluster-a", "Datacenter": "dc-1",
		},
		{
			"VM": "vm-medium", "VM ID": "vm-2", "CPUs": "4", "Memory": "8192",
			"Powerstate": "poweredOn", "Cluster": "cluster-a", "Datacenter": "dc-1",
		},
		{
			"VM": "vm-unknown", "VM ID": "vm-3", "CPUs": "2", "Memory": "4096",
			"Powerstate": "poweredOn", "Cluster": "cluster-b", "Datacenter": "dc-1",
		},
	}
	hosts := []map[string]string{
		{"Host": "host-1", "Cluster": "cluster-a", "Datacenter": "dc-1", "# Cores": "8", "# CPU": "2", "# Memory": "65536"},
		{"Host": "host-2", "Cluster": "cluster-b", "Datacenter": "dc-1", "# Cores": "8", "# CPU": "2", "# Memory": "65536"},
	}
	vDiskHeaders := []string{
		"VM ID", "Disk Key", "Unit #", "Path", "Disk Path", "Capacity MiB",
		"Sharing mode", "Raw", "Shared Bus", "Disk Mode", "Disk UUID",
		"Thin", "Controller", "Label", "SCSI Unit #",
	}
	disks := []map[string]string{
		{"VM ID": "vm-1", "Disk Key": "2000", "Unit #": "0", "Path": "[ds1] vm-easy/disk.vmdk", "Capacity MiB": "2048", "Sharing mode": "false"},
		{"VM ID": "vm-2", "Disk Key": "2001", "Unit #": "0", "Path": "[ds1] vm-medium/disk.vmdk", "Capacity MiB": "10240", "Sharing mode": "false"},
		{"VM ID": "vm-3", "Disk Key": "2002", "Unit #": "0", "Path": "[ds1] vm-unknown/disk.vmdk", "Capacity MiB": "5120", "Sharing mode": "false"},
	}

	sheets := append(defaultStandardSheets(vms, hosts), NewExcelSheet("vDisk", vDiskHeaders, disks))
	f := createTestExcel(t, sheets...)
	ctx := context.Background()
	_, err := parser.IngestRvTools(ctx, f)
	require.NoError(t, err)

	// Manually set OsDiskComplexity: vm-1 → 1 (Easy), vm-2 → 2 (Medium).
	// vm-3 is intentionally left at the DB default (0 = Unknown).
	_, err = parser.db.ExecContext(ctx, `UPDATE vinfo SET "OsDiskComplexity" = 1 WHERE "VM ID" = 'vm-1'`)
	require.NoError(t, err)
	_, err = parser.db.ExecContext(ctx, `UPDATE vinfo SET "OsDiskComplexity" = 2 WHERE "VM ID" = 'vm-2'`)
	require.NoError(t, err)

	inv, err := parser.BuildInventory(ctx)
	require.NoError(t, err)
	require.NotNil(t, inv.VCenter)

	// New enriched field: vm count + disk size per complexity level
	enriched := inv.VCenter.VMs.ComplexityDistribution
	require.Contains(t, enriched, "1")
	require.Contains(t, enriched, "2")
	require.Contains(t, enriched, "0")

	assert.Equal(t, 1, enriched["1"].VMCount)
	assert.InDelta(t, 0.002, enriched["1"].TotalSizeTB, 0.001)
	assert.Equal(t, 1, enriched["2"].VMCount)
	assert.InDelta(t, 0.01, enriched["2"].TotalSizeTB, 0.001)
	assert.Equal(t, 1, enriched["0"].VMCount)
	assert.InDelta(t, 0.005, enriched["0"].TotalSizeTB, 0.001)

	// Old field: derived integer counts only
	dist := inv.VCenter.VMs.DistributionByComplexity
	assert.Equal(t, 1, dist["1"])
	assert.Equal(t, 1, dist["2"])
	assert.Equal(t, 1, dist["0"])

	// Cluster filter: only cluster-a contains vm-1 and vm-2; vm-3 (cluster-b) must be excluded.
	clusterEnriched, err := parser.ComplexityDistribution(ctx, Filters{Cluster: "cluster-a"})
	require.NoError(t, err)
	assert.Contains(t, clusterEnriched, "1")
	assert.Contains(t, clusterEnriched, "2")
	assert.NotContains(t, clusterEnriched, "0")
	assert.Equal(t, 1, clusterEnriched["1"].VMCount)
	assert.Equal(t, 1, clusterEnriched["2"].VMCount)
}

// TestPopulateComplexityQuery_ContainsExpectedWhenClauses verifies that
// PopulateComplexityQuery produces SQL containing a representative sample of
// the WHEN clauses derived from the ComplexityMatrix.
func TestPopulateComplexityQuery_ContainsExpectedWhenClauses(t *testing.T) {
	b := NewBuilder()
	sql, err := b.PopulateComplexityQuery()
	require.NoError(t, err)
	require.NotEmpty(t, sql)

	// Representative WHEN clauses — one from each "corner" of the matrix.
	expectedClauses := []string{
		"o.os_level = 'easy' AND dl.disk_level = 'easy' THEN 1",
		"o.os_level = 'database' AND dl.disk_level = 'whiteglove' THEN 4",
		"o.os_level = 'unsupported' AND dl.disk_level = 'easy' THEN 0",
		"o.os_level = 'medium' AND dl.disk_level = 'hard' THEN 3",
		"o.os_level = 'hard' AND dl.disk_level = 'medium' THEN 2",
	}
	for _, clause := range expectedClauses {
		assert.Contains(t, sql, clause, "SQL should contain WHEN clause: %s", clause)
	}
}

// TestPopulateComplexityQuery_IsDeterministic verifies that calling
// PopulateComplexityQuery twice produces identical SQL output.
func TestPopulateComplexityQuery_IsDeterministic(t *testing.T) {
	b := NewBuilder()
	sql1, err1 := b.PopulateComplexityQuery()
	sql2, err2 := b.PopulateComplexityQuery()

	require.NoError(t, err1)
	require.NoError(t, err2)
	assert.Equal(t, sql1, sql2, "PopulateComplexityQuery output must be deterministic")
}
