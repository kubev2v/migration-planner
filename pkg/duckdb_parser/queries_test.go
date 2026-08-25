package duckdb_parser

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kubev2v/migration-planner/pkg/duckdb_parser/models"
)

// TestVMs_Labels_RVToolsCompatibility verifies that RVTools ingestion still works
// and that VMs get empty labels by default (RVTools Excel files don't have a labels column).
func TestVMs_Labels_RVToolsCompatibility(t *testing.T) {
	parser, _, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()

	vms := []map[string]string{
		{"VM": "vm-1", "VM ID": "vm-001", "VI SDK UUID": "uuid-1", "Host": "esxi-host-1", "CPUs": "4", "Memory": "8192", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"},
		{"VM": "vm-2", "VM ID": "vm-002", "VI SDK UUID": "uuid-2", "Host": "esxi-host-1", "CPUs": "2", "Memory": "4096", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"},
	}
	hosts := []map[string]string{
		{"Datacenter": "dc1", "Cluster": "cluster1", "# Cores": "8", "# CPU": "2", "Object ID": "host-001", "# Memory": "32768", "Model": "ESXi", "Vendor": "VMware", "Host": "esxi-host-1", "Config status": "green"},
	}

	// Create RVTools Excel file (no labels column in RVTools)
	tmpFile := createTestExcel(t, defaultStandardSheets(vms, hosts)...)

	ctx := context.Background()
	_, err := parser.IngestRvTools(ctx, tmpFile)
	require.NoError(t, err, "RVTools ingestion should work even though Excel has no labels column")

	// Verify all VMs have empty labels (default from schema)
	vmsOut, err := parser.VMs(ctx, Filters{}, Options{})
	require.NoError(t, err)
	require.Len(t, vmsOut, 2)

	for _, vm := range vmsOut {
		assert.Empty(t, vm.Labels, "RVTools-ingested VMs should have empty labels array from DEFAULT '[]'")
	}
}

// TestVMs_Labels verifies that the labels field:
// 1. Defaults to empty array for RVTools-ingested VMs
// 2. Can be updated via SQL and is properly returned in the VM model
// 3. Supports multiple labels per VM
func TestVMs_Labels(t *testing.T) {
	parser, _, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()

	vms := []map[string]string{
		{"VM": "vm-1", "VM ID": "vm-001", "VI SDK UUID": "uuid-1", "Host": "esxi-host-1", "CPUs": "4", "Memory": "8192", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"},
		{"VM": "vm-2", "VM ID": "vm-002", "VI SDK UUID": "uuid-2", "Host": "esxi-host-1", "CPUs": "2", "Memory": "4096", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"},
		{"VM": "vm-3", "VM ID": "vm-003", "VI SDK UUID": "uuid-3", "Host": "esxi-host-1", "CPUs": "2", "Memory": "4096", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"},
	}
	hosts := []map[string]string{
		{"Datacenter": "dc1", "Cluster": "cluster1", "# Cores": "8", "# CPU": "2", "Object ID": "host-001", "# Memory": "32768", "Model": "ESXi", "Vendor": "VMware", "Host": "esxi-host-1", "Config status": "green"},
	}

	tmpFile := createTestExcel(t, defaultStandardSheets(vms, hosts)...)

	ctx := context.Background()
	_, err := parser.IngestRvTools(ctx, tmpFile)
	require.NoError(t, err)

	// Verify all VMs default to empty labels array (RVTools behavior)
	vmsOut, err := parser.VMs(ctx, Filters{}, Options{})
	require.NoError(t, err)
	require.Len(t, vmsOut, 3)

	for _, vm := range vmsOut {
		assert.Empty(t, vm.Labels, "RVTools-ingested VMs should default to empty labels array")
	}

	// Simulate agent updating labels via SQL (how the agent will use this)
	_, err = parser.db.ExecContext(ctx, `UPDATE vinfo SET "labels" = '["production", "critical"]' WHERE "VM ID" = 'vm-001'`)
	require.NoError(t, err)

	_, err = parser.db.ExecContext(ctx, `UPDATE vinfo SET "labels" = '["test"]' WHERE "VM ID" = 'vm-002'`)
	require.NoError(t, err)

	// Verify the updates are reflected in VM query
	vmsOut, err = parser.VMs(ctx, Filters{}, Options{})
	require.NoError(t, err)

	vmMap := make(map[string]models.VM)
	for _, vm := range vmsOut {
		vmMap[vm.ID] = vm
	}

	assert.Equal(t, []string{"production", "critical"}, []string(vmMap["vm-001"].Labels), "vm-001 should have two labels")
	assert.Equal(t, []string{"test"}, []string(vmMap["vm-002"].Labels), "vm-002 should have one label")
	assert.Empty(t, vmMap["vm-003"].Labels, "vm-003 should still have empty labels")
}

// TestVMs_FaultToleranceEnabled validates the FT State → FaultToleranceEnabled predicate.
// Covers all vCenter FT states: disabled, notConfigured, enabled, running, primary, secondary, starting, needSecondary, and NULL.
func TestVMs_FaultToleranceEnabled(t *testing.T) {
	parser, _, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()

	vms := []map[string]string{
		// FT State = 'disabled' (FT was on, now off) → should be false
		{"VM": "vm-disabled", "VM ID": "vm-001", "VI SDK UUID": "uuid-1", "Host": "esxi-host-1", "CPUs": "2", "Memory": "4096", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1", "FT State": "disabled"},
		// FT State = 'notConfigured' (never had FT) → should be false
		{"VM": "vm-notconfigured", "VM ID": "vm-002", "VI SDK UUID": "uuid-2", "Host": "esxi-host-1", "CPUs": "2", "Memory": "4096", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1", "FT State": "notConfigured"},
		// Enabled states (all should be true)
		{"VM": "vm-enabled", "VM ID": "vm-003", "VI SDK UUID": "uuid-3", "Host": "esxi-host-1", "CPUs": "2", "Memory": "4096", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1", "FT State": "enabled"},
		{"VM": "vm-running", "VM ID": "vm-004", "VI SDK UUID": "uuid-4", "Host": "esxi-host-1", "CPUs": "2", "Memory": "4096", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1", "FT State": "running"},
		{"VM": "vm-primary", "VM ID": "vm-005", "VI SDK UUID": "uuid-5", "Host": "esxi-host-1", "CPUs": "2", "Memory": "4096", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1", "FT State": "primary"},
		{"VM": "vm-secondary", "VM ID": "vm-006", "VI SDK UUID": "uuid-6", "Host": "esxi-host-1", "CPUs": "2", "Memory": "4096", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1", "FT State": "secondary"},
		{"VM": "vm-starting", "VM ID": "vm-007", "VI SDK UUID": "uuid-7", "Host": "esxi-host-1", "CPUs": "2", "Memory": "4096", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1", "FT State": "starting"},
		{"VM": "vm-needsecondary", "VM ID": "vm-008", "VI SDK UUID": "uuid-8", "Host": "esxi-host-1", "CPUs": "2", "Memory": "4096", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1", "FT State": "needSecondary"},
		// NULL/empty case (no FT State column value) → should be false
		{"VM": "vm-null", "VM ID": "vm-009", "VI SDK UUID": "uuid-9", "Host": "esxi-host-1", "CPUs": "2", "Memory": "4096", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1"},
		// FT State = '' (empty string value) → should be false
		{"VM": "vm-empty", "VM ID": "vm-010", "VI SDK UUID": "uuid-10", "Host": "esxi-host-1", "CPUs": "2", "Memory": "4096", "Powerstate": "poweredOn", "Cluster": "cluster1", "Datacenter": "dc1", "FT State": ""},
	}
	hosts := []map[string]string{
		{"Datacenter": "dc1", "Cluster": "cluster1", "# Cores": "8", "# CPU": "2", "Object ID": "host-001", "# Memory": "32768", "Model": "ESXi", "Vendor": "VMware", "Host": "esxi-host-1", "Config status": "green"},
	}

	tmpFile := createTestExcel(t, defaultStandardSheets(vms, hosts)...)

	ctx := context.Background()
	_, err := parser.IngestRvTools(ctx, tmpFile)
	require.NoError(t, err)

	vmsOut, err := parser.VMs(ctx, Filters{}, Options{})
	require.NoError(t, err)
	require.Len(t, vmsOut, 10)

	vmMap := make(map[string]models.VM)
	for _, vm := range vmsOut {
		vmMap[vm.ID] = vm
	}

	expected := map[string]bool{
		"vm-001": false, // disabled → false
		"vm-002": false, // notConfigured → false
		"vm-003": true,  // enabled → true
		"vm-004": true,  // running → true
		"vm-005": true,  // primary → true
		"vm-006": true,  // secondary → true
		"vm-007": true,  // starting → true
		"vm-008": true,  // needSecondary → true
		"vm-009": false, // NULL → false
		"vm-010": false, // empty string → false
	}
	for id, want := range expected {
		assert.Equal(t, want, vmMap[id].FaultToleranceEnabled, "VM %s: FaultToleranceEnabled", id)
	}
}
