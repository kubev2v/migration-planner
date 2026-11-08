package duckdb_parser

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
)

var hostNICInputs struct {
	rvtools           string
	additionalRVTools string
	withoutNICs       string
	ambiguousHosts    string
	forklift          string
}

func prepareHostNICInputs(t *testing.T) {
	t.Helper()
	hostNICInputs.rvtools = createTestExcel(t,
		NewExcelSheet("vInfo", vInfoHeaders, []map[string]string{
			{"VM ID": "vm-1", "VM": "vm", "Host": "esxi-1", "Cluster": "cluster", "VI SDK UUID": "vc"},
		}),
		NewExcelSheet("vHost", vHostHeaders, []map[string]string{
			{"Host": "esxi-1", "Object ID": "host-1"},
			{"Host": "esxi-2", "Object ID": "host-2"},
		}),
		NewExcelSheet("vNIC", []string{"Host", "Network Device", "Speed"}, []map[string]string{
			{"Host": "esxi-1", "Network Device": "vmnic0", "Speed": "10000"},
			{"Host": "esxi-1", "Network Device": "vmnic1", "Speed": "0"},
			{"Host": "esxi-1", "Network Device": "vmnic2", "Speed": ""},
			{"Host": "esxi-1", "Network Device": "vmnic3", "Speed": "invalid"},
			{"Host": "esxi-2", "Network Device": "vmnic0", "Speed": "25000"},
		}),
	)
	hostNICInputs.additionalRVTools = createTestExcel(t,
		NewExcelSheet("vInfo", vInfoHeaders, []map[string]string{
			{"VM ID": "vm-second", "VM": "second", "Host": "esxi-3", "Cluster": "cluster", "VI SDK UUID": "vc"},
		}),
		NewExcelSheet("vHost", vHostHeaders, []map[string]string{
			{"Host": "esxi-3", "Object ID": "host-3"},
		}),
		NewExcelSheet("vNIC", []string{"Host", "Network Device", "Speed"}, []map[string]string{
			{"Host": "esxi-3", "Network Device": "vmnic0", "Speed": "1000"},
		}),
	)
	hostNICInputs.withoutNICs = createTestExcelWithCustomHeaders(t,
		[]string{"VM ID", "VM", "Cluster", "VI SDK UUID"},
		[]map[string]string{{"VM ID": "vm-2", "VM": "new", "Cluster": "cluster", "VI SDK UUID": "vc"}},
	)
	hostNICInputs.ambiguousHosts = createTestExcel(t,
		NewExcelSheet("vInfo", vInfoHeaders, []map[string]string{
			{"VM ID": "vm-ambiguous", "VM": "ambiguous", "Host": "shared-name", "Cluster": "cluster", "VI SDK UUID": "vc"},
			{"VM ID": "vm-unresolved", "VM": "unresolved", "Host": "missing-host", "Cluster": "cluster", "VI SDK UUID": "vc"},
		}),
		NewExcelSheet("vHost", vHostHeaders, []map[string]string{
			{"Host": "shared-name", "Object ID": "host-10"},
			{"Host": "shared-name", "Object ID": "host-20"},
		}),
		NewExcelSheet("vNIC", []string{"Host", "Network Device", "Speed"}, []map[string]string{
			{"Host": "shared-name", "Network Device": "vmnic0", "Speed": "10000"},
			{"Host": "missing-host", "Network Device": "vmnic1", "Speed": "1000"},
		}),
	)
	hostNICInputs.forklift = createTestSQLite(t, "vc",
		[]sqliteCluster{{id: "cluster", name: "cluster", datacenter: "dc"}},
		[]sqliteVM{{id: "vm-1", name: "vm", clusterName: "cluster"}},
	)

	_, db, cleanup := setupTestParser(t, nil)
	defer cleanup()
	_, err := db.Exec(fmt.Sprintf("ATTACH '%s' AS fixture (TYPE sqlite)", escapeSQLString(hostNICInputs.forklift)))
	require.NoError(t, err)
	_, err = db.Exec(`
        UPDATE fixture.Host SET Network = '{"pNICs":[
            {"key":"vmnic0","linkSpeed":10000},
            {"key":"vmnic1","linkSpeed":0},
            {"key":"vmnic2"},
            {"key":"vmnic3","linkSpeed":null}
        ]}';
        INSERT INTO fixture.Host (ID, Network) VALUES
            ('host-2', '{"pNICs":[{"key":"vmnic0","linkSpeed":25000}]}'),
            ('host-3', '{"pNICs":[]}'),
            ('host-4', '{}'),
            ('host-5', '{"pNICs":null}');
    `)
	require.NoError(t, err)
	_, err = db.Exec("DETACH fixture")
	require.NoError(t, err)
}

var _ = Describe("Host physical NIC ingestion", func() {
	var (
		parser *Parser
		db     *sql.DB
		ctx    context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		db, err = sql.Open("duckdb", "")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(db.Close)
		parser = New(db, nil)
		Expect(parser.Init()).To(Succeed())
		_, err = db.Exec(`CREATE TABLE tag_categories (id VARCHAR, name VARCHAR)`)
		Expect(err).NotTo(HaveOccurred())
	})

	It("keeps ambiguous and missing RVTools host identities unknown", func() {
		result, err := parser.IngestRvTools(ctx, hostNICInputs.ambiguousHosts)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsValid()).To(BeTrue(), "%v", result.Errors)
		var count int
		Expect(db.QueryRow(`SELECT COUNT(*) FROM vnic WHERE "Host ID" IS NULL AND "Host" IN ('shared-name', 'missing-host')`).Scan(&count)).To(Succeed())
		Expect(count).To(Equal(2))
		Expect(db.QueryRow(`SELECT COUNT(*) FROM vinfo WHERE "Host ID" IS NULL`).Scan(&count)).To(Succeed())
		Expect(count).To(Equal(2))
	})

	for _, source := range []string{"RVTools", "Forklift"} {
		Context(source, func() {
			BeforeEach(func() {
				var result ValidationResult
				var err error
				if source == "RVTools" {
					result, err = parser.IngestRvTools(ctx, hostNICInputs.rvtools)
				} else {
					result, err = parser.IngestSqlite(ctx, hostNICInputs.forklift)
				}
				Expect(err).NotTo(HaveOccurred())
				Expect(result.IsValid()).To(BeTrue(), "%v", result.Errors)
			})

			It("retains host and NIC identities and stores unavailable speeds as NULL", func() {
				rows, err := db.Query(`SELECT "Host ID", "Network Device", "Speed" FROM vnic ORDER BY "Host ID", "Network Device"`)
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(rows.Close)

				type record struct {
					host, id string
					speed    sql.NullInt32
				}
				var got []record
				for rows.Next() {
					var nic record
					Expect(rows.Scan(&nic.host, &nic.id, &nic.speed)).To(Succeed())
					got = append(got, nic)
				}
				Expect(rows.Err()).NotTo(HaveOccurred())
				Expect(got).To(Equal([]record{
					{"host-1", "vmnic0", sql.NullInt32{Int32: 10000, Valid: true}},
					{"host-1", "vmnic1", sql.NullInt32{}},
					{"host-1", "vmnic2", sql.NullInt32{}},
					{"host-1", "vmnic3", sql.NullInt32{}},
					{"host-2", "vmnic0", sql.NullInt32{Int32: 25000, Valid: true}},
				}))
			})

			It("retains the VM's owning host ID", func() {
				var hostID string
				Expect(db.QueryRow(`SELECT "Host ID" FROM vinfo WHERE "VM ID" = 'vm-1'`).Scan(&hostID)).To(Succeed())
				Expect(hostID).To(Equal("host-1"))
			})

			It("clears collected NICs before an RVTools import without a vNIC sheet", func() {
				Expect(parser.Cleanup()).To(Succeed())
				result, err := parser.IngestRvTools(ctx, hostNICInputs.withoutNICs)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.IsValid()).To(BeTrue(), "%v", result.Errors)

				var count int
				Expect(db.QueryRow(`SELECT COUNT(*) FROM vnic`).Scan(&count)).To(Succeed())
				Expect(count).To(BeZero())
			})

			if source == "Forklift" {
				It("does not create NIC records for empty, missing, or null pNIC lists", func() {
					var count int
					Expect(db.QueryRow(`SELECT COUNT(*) FROM vnic WHERE "Host ID" IN ('host-3', 'host-4', 'host-5')`).Scan(&count)).To(Succeed())
					Expect(count).To(BeZero())
				})
			}

			if source == "RVTools" {
				It("keeps existing NICs when importing another workbook", func() {
					result, err := parser.IngestRvTools(ctx, hostNICInputs.additionalRVTools)
					Expect(err).NotTo(HaveOccurred())
					Expect(result.IsValid()).To(BeTrue(), "%v", result.Errors)

					var count, speed int
					Expect(db.QueryRow(`SELECT COUNT(*) FROM vnic`).Scan(&count)).To(Succeed())
					Expect(count).To(Equal(6))
					Expect(db.QueryRow(`SELECT "Speed" FROM vnic WHERE "Host ID" = 'host-3'`).Scan(&speed)).To(Succeed())
					Expect(speed).To(Equal(1000))
				})
			}
		})
	}
})
