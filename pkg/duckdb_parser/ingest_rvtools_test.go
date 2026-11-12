package duckdb_parser

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kubev2v/migration-planner/pkg/inventory"
)

func TestIngestRvTools_Annotation(t *testing.T) {
	parser, _, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()

	const annotation = "  SAP production application: 東京 / Finance!\nOwner: Финансы.  "
	headers := []string{"VM ID", "VM", "Annotation", "Cluster", "VI SDK UUID"}
	vms := []map[string]string{
		{"VM ID": "vm-annotated", "VM": "annotated-vm", "Annotation": annotation, "Cluster": "cluster-1", "VI SDK UUID": "vcenter-1"},
		{"VM ID": "vm-unannotated", "VM": "unannotated-vm", "Cluster": "cluster-1", "VI SDK UUID": "vcenter-1"},
	}
	workbook := createTestExcelWithCustomHeaders(t, headers, vms)

	ctx := context.Background()
	result, err := parser.IngestRvTools(ctx, workbook)
	require.NoError(t, err)
	require.True(t, result.IsValid(), "RVTools validation failed: %v", result.Errors)

	var got sql.NullString
	err = parser.db.QueryRowContext(ctx, `SELECT "Annotation" FROM vinfo WHERE "VM ID" = ?`, "vm-annotated").Scan(&got)
	require.NoError(t, err)
	require.True(t, got.Valid)
	require.Equal(t, annotation, got.String)

	err = parser.db.QueryRowContext(ctx, `SELECT "Annotation" FROM vinfo WHERE "VM ID" = ?`, "vm-unannotated").Scan(&got)
	require.NoError(t, err)
	require.True(t, !got.Valid || got.String == "", "unannotated VM has unexpected annotation: %q", got.String)
}

func TestIngestRvTools_MissingAnnotationColumn(t *testing.T) {
	parser, _, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()

	headers := []string{"VM ID", "VM", "Cluster", "VI SDK UUID"}
	vms := []map[string]string{
		{"VM ID": "vm-legacy", "VM": "legacy-vm", "Cluster": "cluster-1", "VI SDK UUID": "vcenter-1"},
	}
	workbook := createTestExcelWithCustomHeaders(t, headers, vms)

	ctx := context.Background()
	result, err := parser.IngestRvTools(ctx, workbook)
	require.NoError(t, err)
	require.True(t, result.IsValid(), "RVTools validation failed: %v", result.Errors)

	var got sql.NullString
	err = parser.db.QueryRowContext(ctx, `SELECT "Annotation" FROM vinfo WHERE "VM ID" = ?`, "vm-legacy").Scan(&got)
	require.NoError(t, err, "VM must be ingested even when Annotation is absent")
	require.False(t, got.Valid)
}

func TestIngestRvTools_HostFacts(t *testing.T) {
	tests := []struct {
		name     string
		headers  []string
		values   map[string]string
		cpuModel string
		serial   string
	}{
		{
			name:     "preserves optional host facts without Object ID",
			headers:  []string{"Datacenter", "Cluster", "# Cores", "# CPU", "# Memory", "Model", "Vendor", "Host", "Config status", "VMotion support", "Storage VMotion support", "CPU Model", "Serial number", "Service tag"},
			values:   map[string]string{"Datacenter": "東京 DC", "Cluster": "cluster-a", "# Cores": "32", "# CPU": "2", "# Memory": "262144", "Model": "PowerEdge", "Vendor": "Dell", "Host": "esx-a", "CPU Model": " Xeon Gold 6338 ", "Serial number": " SN-1 ", "Service tag": "TAG-1"},
			cpuModel: "Xeon Gold 6338",
			serial:   "SN-1",
		},
		{
			name:    "missing identity and hardware columns remain unknown",
			headers: []string{"Datacenter", "Cluster", "# Cores", "# CPU", "# Memory", "Model", "Vendor", "Host"},
			values:  map[string]string{"Datacenter": "dc-b", "Cluster": "cluster-a", "# Cores": "8", "# CPU": "1", "# Memory": "32768", "Model": "Server", "Vendor": "Vendor", "Host": "esx-b"},
		},
		{
			name:     "service tag fills an absent serial number",
			headers:  []string{"Datacenter", "Cluster", "# Cores", "# CPU", "# Memory", "Model", "Vendor", "Host", "CPU Model", "Service tag"},
			values:   map[string]string{"Datacenter": "dc-c", "Cluster": "cluster-a", "# Cores": "16", "# CPU": "1", "# Memory": "65536", "Model": "Server", "Vendor": "Vendor", "Host": "esx-c", "CPU Model": "EPYC", "Service tag": " TAG-3 "},
			cpuModel: "EPYC",
			serial:   "TAG-3",
		},
		{
			name:    "RVTools VM placeholders normalize to unknown",
			headers: []string{"Datacenter", "Cluster", "# Cores", "# CPU", "# Memory", "Model", "Vendor", "Host", "CPU Model", "Serial number", "Service tag"},
			values:  map[string]string{"Datacenter": "dc-d", "Cluster": "cluster-a", "# Cores": "16", "# CPU": "1", "# Memory": "65536", "Model": "Server", "Vendor": "Vendor", "Host": "esx-d", "CPU Model": "vM", "Serial number": "VM", "Service tag": " "},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, _, cleanup := setupTestParser(t, &testValidator{})
			defer cleanup()

			workbook := createTestExcel(t,
				NewExcelSheet("vInfo", vInfoHeaders, []map[string]string{{"VM": "vm", "VM ID": "vm-1", "VI SDK UUID": "vc-1", "Cluster": "cluster-a"}}),
				NewExcelSheet("vHost", tt.headers, []map[string]string{tt.values}),
			)
			result, err := parser.IngestRvTools(context.Background(), workbook)
			require.NoError(t, err)
			require.True(t, result.IsValid(), "RVTools validation failed: %v", result.Errors)

			hosts, err := parser.Hosts(context.Background(), Filters{}, Options{})
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, tt.values["Datacenter"], hosts[0].Datacenter)
			assert.Equal(t, tt.values["Cluster"], hosts[0].Cluster)
			assert.Equal(t, tt.cpuModel, hosts[0].CpuModel)
			assert.Equal(t, tt.serial, hosts[0].SerialNumber)
			assert.Empty(t, hosts[0].Id)

			built, err := parser.BuildInventory(context.Background(), nil)
			require.NoError(t, err)
			require.NotNil(t, built.VCenter)
			require.Len(t, built.VCenter.Infra.Hosts, 1)
			assert.Equal(t, tt.cpuModel, built.VCenter.Infra.Hosts[0].CpuModel)
			assert.Equal(t, tt.serial, built.VCenter.Infra.Hosts[0].SerialNumber)
			var clusterHosts []inventory.Host
			for _, cluster := range built.Clusters {
				clusterHosts = append(clusterHosts, cluster.Infra.Hosts...)
			}
			require.Len(t, clusterHosts, 1)
			assert.Equal(t, tt.cpuModel, clusterHosts[0].CpuModel)
			assert.Equal(t, tt.serial, clusterHosts[0].SerialNumber)
		})
	}
}
