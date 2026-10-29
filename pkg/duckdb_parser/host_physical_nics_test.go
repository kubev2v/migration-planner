package duckdb_parser

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHostPhysicalNICIngestion(t *testing.T) {
	for _, source := range []string{"rvtools", "forklift"} {
		t.Run(source, func(t *testing.T) {
			parser, db, cleanup := setupTestParser(t, nil)
			defer cleanup()
			ctx := context.Background()
			var result ValidationResult
			var err error
			if source == "rvtools" {
				workbook := createTestExcel(t, []ExcelSheet{
					NewExcelSheet("vInfo", vInfoHeaders, []map[string]string{{"VM ID": "vm-1", "VM": "vm", "Host": "host-1", "Cluster": "cluster", "VI SDK UUID": "vc"}}),
					NewExcelSheet("vNIC", []string{"Host", "Network Device", "Speed"}, []map[string]string{
						{"Host": "host-1", "Network Device": "vmnic0", "Speed": "10000"},
						{"Host": "host-1", "Network Device": "vmnic1", "Speed": "0"},
						{"Host": "host-1", "Network Device": "vmnic2", "Speed": ""},
						{"Host": "host-1", "Network Device": "vmnic3", "Speed": "invalid"},
						{"Host": "host-2", "Network Device": "vmnic0", "Speed": "25000"},
					}),
				}...)
				result, err = parser.IngestRvTools(ctx, workbook)
			} else {
				path := createTestSQLite(t, "vc", []sqliteCluster{{id: "cluster", name: "cluster", datacenter: "dc"}}, []sqliteVM{{id: "vm-1", name: "vm", clusterName: "cluster"}})
				_, err = db.Exec(fmt.Sprintf("ATTACH '%s' AS fixture (TYPE sqlite)", escapeSQLString(path)))
				require.NoError(t, err)
				_, err = db.Exec(`UPDATE fixture.Host SET Network = '{"pNICs":[{"key":"vmnic0","linkSpeed":10000},{"key":"vmnic1","linkSpeed":0},{"key":"vmnic2"},{"key":"vmnic3","linkSpeed":null}]}';
      INSERT INTO fixture.Host (ID, Network) VALUES ('host-2', '{"pNICs":[{"key":"vmnic0","linkSpeed":25000}]}'), ('host-3', '{"pNICs":[]}'), ('host-4', '{}')`)
				require.NoError(t, err)
				_, err = db.Exec("DETACH fixture")
				require.NoError(t, err)
				result, err = parser.IngestSqlite(ctx, path)
			}
			require.NoError(t, err)
			require.True(t, result.IsValid(), "%v", result.Errors)
			rows, err := db.Query(`SELECT "Host", "Network Device", "Speed" FROM vnic ORDER BY "Host", "Network Device"`)
			require.NoError(t, err)
			type record struct {
				host, id string
				speed    sql.NullInt32
			}
			var got []record
			for rows.Next() {
				var r record
				require.NoError(t, rows.Scan(&r.host, &r.id, &r.speed))
				got = append(got, r)
			}
			require.NoError(t, rows.Err())
			require.NoError(t, rows.Close())
			require.Equal(t, []record{
				{"host-1", "vmnic0", sql.NullInt32{Int32: 10000, Valid: true}},
				{"host-1", "vmnic1", sql.NullInt32{}},
				{"host-1", "vmnic2", sql.NullInt32{}},
				{"host-1", "vmnic3", sql.NullInt32{}},
				{"host-2", "vmnic0", sql.NullInt32{Int32: 25000, Valid: true}},
			}, got)
			if source == "rvtools" {
				second := createTestExcel(t,
					NewExcelSheet("vInfo", vInfoHeaders, []map[string]string{{"VM ID": "vm-second", "VM": "second", "Host": "host-3", "Cluster": "cluster", "VI SDK UUID": "vc"}}),
					NewExcelSheet("vNIC", []string{"Host", "Network Device", "Speed"}, []map[string]string{{"Host": "host-3", "Network Device": "vmnic0", "Speed": "1000"}}),
				)
				result, err = parser.IngestRvTools(ctx, second)
				require.NoError(t, err)
				require.True(t, result.IsValid())
				var total, speed int
				require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM vnic`).Scan(&total))
				require.Equal(t, 6, total)
				require.NoError(t, db.QueryRow(`SELECT "Speed" FROM vnic WHERE "Host" = 'host-3'`).Scan(&speed))
				require.Equal(t, 1000, speed)
			}
			require.NoError(t, parser.Cleanup())
			workbook := createTestExcelWithCustomHeaders(t, []string{"VM ID", "VM", "Cluster", "VI SDK UUID"}, []map[string]string{{"VM ID": "vm-2", "VM": "new", "Cluster": "cluster", "VI SDK UUID": "vc"}})
			result, err = parser.IngestRvTools(ctx, workbook)
			require.NoError(t, err)
			require.True(t, result.IsValid())
			var count int
			require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM vnic`).Scan(&count))
			require.Zero(t, count)
		})
	}
}
