package duckdb_parser

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNICCountDefault(t *testing.T) {
	_, db, cleanup := setupTestParser(t, nil)
	defer cleanup()
	_, err := db.Exec(`INSERT INTO vinfo ("VM ID", "VM") VALUES ('vm-default', 'default-vm')`)
	require.NoError(t, err)
	var count int32
	require.NoError(t, db.QueryRow(`SELECT "NICs" FROM vinfo WHERE "VM ID"='vm-default'`).Scan(&count))
	require.Zero(t, count)
}

func TestIngestRvTools_NICInventory(t *testing.T) {
	parser, db, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()
	vms := []map[string]string{{"VM": "many-nics", "VM ID": "vm-1", "Cluster": "cluster-1", "VI SDK UUID": "vc-1", "NICs": "10"}}
	sheets := defaultStandardSheets(vms, nil)
	headers := append(append([]string{}, vInfoHeaders...), "NICs")
	sheets[0] = NewExcelSheet("vInfo", headers, vms)
	nicHeaders := []string{"VM ID", "Network", "Mac Address", "NIC label", "Adapter", "Switch", "Connected", "Starts Connected", "Type", "IPv4 Address", "IPv6 Address", "Cluster"}
	var nics []map[string]string
	for i := 0; i < 10; i++ {
		nics = append(nics, map[string]string{
			"VM ID": "vm-1", "Network": "Production", "Mac Address": fmt.Sprintf("00:50:56:00:00:%02x", i),
			"NIC label": fmt.Sprintf("Network adapter %d", i+1), "Adapter": "Vmxnet3", "Switch": "switch-1",
			"Connected": "false", "Starts Connected": "false", "Type": "distributed", "Cluster": "cluster-1",
		})
	}
	sheets = append(sheets, NewExcelSheet("vNetwork", nicHeaders, nics))
	result, err := parser.IngestRvTools(context.Background(), createTestExcel(t, sheets...))
	require.NoError(t, err)
	require.True(t, result.IsValid(), "%v", result.Errors)
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM vnetwork WHERE "VM ID"='vm-1' AND "Adapter"='Vmxnet3' AND "Network"='Production' AND NOT "Connected" AND NOT "Starts Connected"`).Scan(&count))
	require.Equal(t, 10, count, "separate NICs sharing one network must survive, including more than eight")
	require.NoError(t, db.QueryRow(`SELECT "NICs" FROM vinfo WHERE "VM ID"='vm-1'`).Scan(&count))
	require.Equal(t, 10, count)
	vmsOut, err := parser.VMs(context.Background(), Filters{}, Options{})
	require.NoError(t, err)
	require.Len(t, vmsOut, 1)
	require.Len(t, vmsOut[0].NICs, 10)
	require.Equal(t, "Vmxnet3", vmsOut[0].NICs[0].Adapter)
	require.Equal(t, "Production", vmsOut[0].NICs[0].Network.ID)
}

func TestIngestRvTools_NICCountWithoutNetworkSheet(t *testing.T) {
	parser, db, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()
	vms := []map[string]string{{"VM": "count-only", "VM ID": "vm-1", "Cluster": "cluster-1", "VI SDK UUID": "vc-1", "NICs": "3"}}
	sheets := defaultStandardSheets(vms, nil)
	sheets[0] = NewExcelSheet("vInfo", append(append([]string{}, vInfoHeaders...), "NICs"), vms)
	result, err := parser.IngestRvTools(context.Background(), createTestExcel(t, sheets...))
	require.NoError(t, err)
	require.True(t, result.IsValid())
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM vnetwork`).Scan(&count))
	require.Zero(t, count, "an absent detail sheet cannot establish that the VM has zero adapters")
	found := false
	for _, warning := range result.Warnings {
		found = found || warning.Code == CodeEmptyNICs
	}
	require.True(t, found)
	require.NoError(t, db.QueryRow(`SELECT "NICs" FROM vinfo WHERE "VM ID"='vm-1'`).Scan(&count))
	require.Equal(t, 3, count)
}

func TestIngestRvTools_UnknownAndZeroNICCounts(t *testing.T) {
	parser, db, cleanup := setupTestParser(t, &testValidator{})
	defer cleanup()
	var vms []map[string]string
	for _, value := range []string{"0", "", "VM"} {
		vms = append(vms, map[string]string{"VM": "vm-" + value, "VM ID": "vm-" + value, "Cluster": "cluster-1", "VI SDK UUID": "vc-1", "NICs": value})
	}
	sheets := defaultStandardSheets(vms, nil)
	sheets[0] = NewExcelSheet("vInfo", append(append([]string{}, vInfoHeaders...), "NICs"), vms)
	result, err := parser.IngestRvTools(context.Background(), createTestExcel(t, sheets...))
	require.NoError(t, err)
	require.True(t, result.IsValid())
	for _, vm := range vms {
		var count int32
		require.NoError(t, db.QueryRow(`SELECT "NICs" FROM vinfo WHERE "VM ID"=?`, vm["VM ID"]).Scan(&count))
		require.Zero(t, count)
	}
}
