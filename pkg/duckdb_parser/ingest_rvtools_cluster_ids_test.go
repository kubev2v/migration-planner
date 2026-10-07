package duckdb_parser

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
)

func clusterIDExcelSheets(firstVM, firstCluster string) []ExcelSheet {
	return []ExcelSheet{
		NewExcelSheet("vInfo", vInfoHeaders, []map[string]string{
			{"VM": firstVM, "VM ID": firstVM, "Cluster": firstCluster,
				"Datacenter": "dc-a", "VI SDK UUID": "vc-a", "CPUs": "2",
				"Memory": "4096", "Powerstate": "poweredOn", "Template": "false"},
		}),
	}
}

func TestIngestRvTools_ClusterIDs(t *testing.T) {
	cases := []struct {
		name         string
		clusterSheet *ExcelSheet
	}{
		{name: "missing sheet"},
		{name: "empty cell", clusterSheet: &ExcelSheet{
			Name: "vCluster", Headers: vClusterHeaders,
			Rows: []map[string]string{{"Name": "cluster-a", "DRS enabled": "true", "HA enabled": "true"}},
		}},
		{name: "VM placeholder", clusterSheet: &ExcelSheet{
			Name: "vCluster", Headers: vClusterHeaders,
			Rows: []map[string]string{{"Name": "cluster-a", "Object ID": "VM", "DRS enabled": "true", "HA enabled": "true"}},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, db, _ := newClusterIDTestParser(t)
			sheets := clusterIDExcelSheets("vm-a", "cluster-a")
			if tc.clusterSheet != nil {
				sheets = append(sheets, *tc.clusterSheet)
			}
			ctx := context.Background()
			result, err := p.IngestRvTools(ctx, createTestExcel(t, sheets...))
			require.NoError(t, err)
			require.False(t, result.HasErrors(), "%+v", result.Errors)
			inv, err := p.BuildInventory(ctx, nil)
			require.NoError(t, err)
			expected := generateClusterID("cluster-a", "dc-a", "vc-a")
			var id string
			var drs, ha sql.NullBool
			require.NoError(t, db.QueryRow(`SELECT "Object ID", "DrsEnabled", "DasEnabled"
                FROM vcluster WHERE "Name" = 'cluster-a'`).Scan(&id, &drs, &ha))
			require.Equal(t, expected, id)
			require.Equal(t, tc.clusterSheet != nil, drs.Valid)
			require.Equal(t, tc.clusterSheet != nil, ha.Valid)
			require.Equal(t, tc.clusterSheet != nil, drs.Bool)
			require.Equal(t, tc.clusterSheet != nil, ha.Bool)
			require.Contains(t, inv.Clusters, expected)
			require.Equal(t, 1, inv.Clusters[expected].VMs.Total)
			// A subset rebuild must use the persisted ID without adding rows.
			subset, err := p.BuildInventory(ctx, []string{"vm-a"})
			require.NoError(t, err)
			require.Contains(t, subset.Clusters, expected)
			if tc.clusterSheet == nil {
				require.Nil(t, inv.Clusters[expected].ClusterFeatures)
				require.Nil(t, subset.Clusters[expected].ClusterFeatures)
			} else {
				require.NotNil(t, inv.Clusters[expected].ClusterFeatures)
				require.NotNil(t, subset.Clusters[expected].ClusterFeatures)
				require.NotNil(t, subset.Clusters[expected].ClusterFeatures.DrsEnabled)
				require.NotNil(t, subset.Clusters[expected].ClusterFeatures.HaEnabled)
				require.True(t, *subset.Clusters[expected].ClusterFeatures.DrsEnabled)
				require.True(t, *subset.Clusters[expected].ClusterFeatures.HaEnabled)
			}
			var n int
			require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM vcluster`).Scan(&n))
			require.Equal(t, 1, n)
		})
	}
}
