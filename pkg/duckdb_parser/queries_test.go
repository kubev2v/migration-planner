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
