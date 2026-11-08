package duckdb_parser

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/kubev2v/migration-planner/pkg/duckdb_parser/models"
	"github.com/kubev2v/migration-planner/pkg/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
)

var _ = Describe("VM metadata", func() {
	Context("SQLite tags and custom attributes", func() {
		var (
			ctx  context.Context
			path string
			db   *sql.DB
		)

		BeforeEach(func() {
			t := GinkgoTB()

			ctx = context.Background()
			path = createTestSQLite(t, "uuid", []sqliteCluster{{id: "c1", name: "cluster", datacenter: "dc"}},
				[]sqliteVM{{id: "vm1", name: "vm", clusterName: "cluster"}})

			var err error
			db, err = sql.Open("duckdb", "")
			Expect(err).To(Succeed())
			DeferCleanup(db.Close)
			_, err = db.Exec(`ATTACH '` + escapeSQLString(path) + `' AS src (TYPE sqlite);
		ALTER TABLE src.VM ADD COLUMN Tags VARCHAR;
		ALTER TABLE src.VM ADD COLUMN CustomDef VARCHAR;
		ALTER TABLE src.VM ADD COLUMN CustomValues VARCHAR`)
			Expect(err).To(Succeed())
			_, err = db.Exec(`UPDATE src.VM SET Tags = ?, CustomDef = ?, CustomValues = ?`,
				`[{"categoryID":"cat1","name":"Payments; QA"},{"categoryID":"cat1","name":"Production"},{"categoryID":"cat1","name":"Payments; QA"},{"categoryID":"cat2","name":"QA"}]`,
				`[{"key":1,"name":"cat1"},{"key":2,"name":"Owner's; \"name\""},{"key":3,"name":"Empty"}]`,
				`[{"key":1,"value":"  Finance & IT  "},{"key":2,"value":"HR"},{"key":3,"value":" "},{"key":99,"value":"unmatched"}]`)
			Expect(err).To(Succeed())
			_, err = db.Exec(`DETACH src`)
			Expect(err).To(Succeed())
		})

		It("imports tags and custom attributes", func() {
			t := GinkgoTB()

			p, _, cleanup := setupTestParser(t, nil)
			defer cleanup()
			_, err := p.db.ExecContext(ctx, `INSERT INTO tag_categories VALUES (?, ?)`, "cat1", "Owner's; \"name\"")
			Expect(err).To(Succeed())
			result, err := p.IngestSqlite(ctx, path)
			Expect(err).To(Succeed())
			Expect(result.HasErrors()).To(BeFalse())
			var temporaryTables int
			Expect(p.db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_catalog = 'temp' AND table_name = 'tag_categories'`).Scan(&temporaryTables)).To(Succeed())
			Expect(temporaryTables).To(BeZero(), "ingestion must not create a temporary category table")
			var categoryID string
			Expect(p.db.QueryRowContext(ctx, `SELECT id FROM tag_categories`).Scan(&categoryID)).To(Succeed())
			Expect(categoryID).To(Equal("cat1"), "ingestion must preserve the category lookup table")
			vms, err := p.VMs(ctx, Filters{}, Options{})
			Expect(err).To(Succeed())
			Expect(vms).To(HaveLen(1))
			Expect(vms[0].Metadata).To(Equal(models.Metadata{
				"Owner's; \"name\"": {"HR", "Payments; QA", "Production"},
				"cat1":              {"  Finance & IT  "},
			}))
			var count int
			Expect(p.db.QueryRowContext(ctx, `SELECT count(*) FROM vm_metadata WHERE vm_id = 'vm1'`).Scan(&count)).To(Succeed())
			Expect(count).To(Equal(4))
			Expect(p.db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_name='vinfo' AND column_name='metadata'`).Scan(&count)).To(Succeed())
			Expect(count).To(BeZero())
			Expect(vms[0].Labels).To(BeEmpty())
		})

		DescribeTable("omits unresolved tags without losing custom attributes",
			func(categoryName string) {
				t := GinkgoTB()

				unresolved, _, closeUnresolved := setupTestParser(t, nil)
				defer closeUnresolved()
				if categoryName != "unavailable" {
					_, err := unresolved.db.ExecContext(ctx, `INSERT INTO tag_categories VALUES ('cat1', ?)`, categoryName)
					Expect(err).To(Succeed())
				}
				result, err := unresolved.IngestSqlite(ctx, path)
				Expect(err).To(Succeed())
				Expect(result.HasErrors()).To(BeFalse())
				vms, err := unresolved.VMs(ctx, Filters{}, Options{})
				Expect(err).To(Succeed())
				Expect(vms).To(HaveLen(1))
				Expect(vms[0].Metadata).To(Equal(models.Metadata{
					"cat1": {"  Finance & IT  "}, "Owner's; \"name\"": {"HR"},
				}), "unresolved tags must be omitted without losing custom attributes")
			},
			Entry("empty category name", ""),

			Entry("blank category name", " "),

			Entry("missing category", "unavailable"),
		)

		It("keeps aggregate inventory unchanged by metadata", func() {
			t := GinkgoTB()

			p, _, cleanup := setupTestParser(t, nil)
			defer cleanup()
			_, err := p.db.ExecContext(ctx, `INSERT INTO tag_categories VALUES (?, ?)`, "cat1", "Owner's; \"name\"")
			Expect(err).To(Succeed())

			result, err := p.IngestSqlite(ctx, path)
			Expect(err).To(Succeed())
			Expect(result.HasErrors()).To(BeFalse())
			withMetadata, err := p.BuildInventory(ctx, nil)
			Expect(err).To(Succeed())

			_, err = db.Exec(`ATTACH '` + escapeSQLString(path) + `' AS src (TYPE sqlite);
		UPDATE src.VM SET Tags = '[]', CustomValues = '[]'; DETACH src`)
			Expect(err).To(Succeed())
			fresh, _, closeFresh := setupTestParser(t, nil)
			defer closeFresh()
			_, err = fresh.IngestSqlite(ctx, path)
			Expect(err).To(Succeed())
			vms, err := fresh.VMs(ctx, Filters{}, Options{})
			Expect(err).To(Succeed())
			Expect(vms[0].Metadata).To(BeEmpty())
			withoutMetadata, err := fresh.BuildInventory(ctx, nil)
			Expect(err).To(Succeed())
			withMetadata.CreatedAt, withoutMetadata.CreatedAt = nil, nil
			Expect(withoutMetadata).To(Equal(withMetadata), "metadata must not change aggregate inventory")
		})
	})

	It("stops on malformed SQLite metadata and permits a clean retry", func() {
		t := GinkgoTB()

		path := createTestSQLite(t, "uuid", []sqliteCluster{{id: "c1", name: "cluster", datacenter: "dc"}},
			[]sqliteVM{{id: "vm1", name: "vm", clusterName: "cluster"}})
		db, err := sql.Open("duckdb", "")
		Expect(err).To(Succeed())
		DeferCleanup(db.Close)
		_, err = db.Exec(`ATTACH '` + escapeSQLString(path) + `' AS src (TYPE sqlite);
		ALTER TABLE src.VM ADD COLUMN Tags VARCHAR;
		UPDATE src.VM SET Tags = 'invalid JSON'; DETACH src`)
		Expect(err).To(Succeed())
		p, _, cleanup := setupTestParser(t, nil)
		defer cleanup()
		_, err = p.db.ExecContext(context.Background(), `INSERT INTO tag_categories VALUES ('cat1', 'Owner')`)
		Expect(err).To(Succeed())
		_, err = p.IngestSqlite(context.Background(), path)
		Expect(err).To(MatchError(ContainSubstring("Malformed JSON")))
		var count int
		Expect(p.db.QueryRowContext(context.Background(), `SELECT count(*) FROM vinfo`).Scan(&count)).To(Succeed())
		Expect(count).To(Equal(1), "VM insert precedes metadata, but failed ingestion must not be published")
		Expect(p.db.QueryRowContext(context.Background(), `SELECT count(*) FROM vcpu`).Scan(&count)).To(Succeed())
		Expect(count).To(BeZero(), "metadata failure must stop later inventory writes")
		Expect(p.db.QueryRowContext(context.Background(), `SELECT count(*) FROM duckdb_databases() WHERE database_name = 'src'`).Scan(&count)).To(Succeed())
		Expect(count).To(BeZero(), "failed import must detach its SQLite source")
		Expect(p.db.QueryRowContext(context.Background(), `SELECT count(*) FROM tag_categories`).Scan(&count)).To(Succeed())
		Expect(count).To(Equal(1), "failed import must preserve category names for retry")
		validPath := createTestSQLite(t, "uuid", []sqliteCluster{{id: "c1", name: "cluster", datacenter: "dc"}},
			[]sqliteVM{{id: "vm2", name: "replacement", clusterName: "cluster"}})
		_, err = p.IngestSqlite(context.Background(), validPath)
		Expect(err).To(Succeed())
		var name string
		Expect(p.db.QueryRowContext(context.Background(), `SELECT "VM" FROM vinfo WHERE "VM ID" = 'vm2'`).Scan(&name)).To(Succeed())
		Expect(name).To(Equal("replacement"))
	})

	It("preserves category names when retrying a cancelled SQLite import", func() {
		t := GinkgoTB()

		p, db, cleanup := setupTestParser(t, nil)
		defer cleanup()
		path := createTestSQLite(t, "uuid", []sqliteCluster{{id: "c1", name: "cluster", datacenter: "dc"}},
			[]sqliteVM{{id: "vm1", name: "vm", clusterName: "cluster"}})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		_, err := db.ExecContext(ctx, `INSERT INTO tag_categories VALUES ('cat1', 'Owner')`)
		Expect(err).To(Succeed())
		p.db = &cancelVMWriteDB{DB: db, cancel: cancel}
		_, err = p.IngestSqlite(ctx, path)
		Expect(err).To(MatchError(context.Canceled))
		var count int
		Expect(db.QueryRowContext(context.Background(), `SELECT count(*) FROM tag_categories`).Scan(&count)).To(Succeed())
		Expect(count).To(Equal(1))
		Expect(db.QueryRowContext(context.Background(), `SELECT count(*) FROM duckdb_databases() WHERE database_name = 'src'`).Scan(&count)).To(Succeed())
		Expect(count).To(BeZero())
		result, err := p.IngestSqlite(context.Background(), path)
		Expect(err).To(Succeed())
		Expect(result.HasErrors()).To(BeFalse(), "retry must import a valid inventory")

		var name string
		Expect(db.QueryRow(`SELECT "VM" FROM vinfo WHERE "VM ID" = 'vm1'`).Scan(&name)).To(Succeed())
		Expect(name).To(Equal("vm"), "retry must import the original VM")

		var categoryName string
		Expect(db.QueryRow(`SELECT name FROM tag_categories WHERE id = 'cat1'`).Scan(&categoryName)).To(Succeed())
		Expect(categoryName).To(Equal("Owner"), "retry must preserve the category lookup")
	})

	It("retries a cancelled RVTools import after cleanup", func() {
		t := GinkgoTB()

		p, db, cleanup := setupTestParser(t, nil)
		defer cleanup()
		sheets := defaultStandardSheets([]map[string]string{{"VM": "vm", "VM ID": "vm1", "VI SDK UUID": "uuid",
			"Host": "host", "Cluster": "cluster", "Datacenter": "dc", "Owner": "Finance"}},
			[]map[string]string{{"Host": "host", "Cluster": "cluster", "Datacenter": "dc"}})
		sheets[0].Headers = []string{"VM", "VM ID", "VI SDK UUID", "Host", "Cluster", "CPUs", "Memory", "Powerstate", "Template",
			"Annotation", "Owner", "Datacenter"}
		path := createTestExcel(t, sheets...)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		p.db = &cancelVMWriteDB{DB: db, cancel: cancel}
		_, err := p.IngestRvTools(ctx, path)
		Expect(err).To(MatchError(context.Canceled))
		Expect(p.Cleanup()).To(Succeed())
		_, err = p.IngestRvTools(context.Background(), path)
		Expect(err).To(Succeed())
		Expect(storedMetadata(t, db, "vm1")).To(Equal(map[string][]string{"Owner": {"Finance"}}))
	})

	Context("SQLite metadata for existing VMs", func() {
		cases := []struct {
			template        bool
			owner           string
			differentCenter bool
		}{
			{template: false, owner: "Finance"},

			{template: true, owner: "Finance"},

			{template: false, owner: ""},

			{template: false, owner: "Finance", differentCenter: true},
		}

		for _, tc := range cases {
			It(fmt.Sprintf("incoming template=%t, existing owner=%q, different vCenter=%t", tc.template, tc.owner, tc.differentCenter), func() {
				t := GinkgoTB()

				ctx := context.Background()
				p, _, cleanup := setupTestParser(t, nil)
				defer cleanup()
				_, err := p.db.ExecContext(ctx, `INSERT INTO tag_categories VALUES ('Owner', 'Owner')`)
				Expect(err).To(Succeed())
				db, err := sql.Open("duckdb", "")
				Expect(err).To(Succeed())
				DeferCleanup(db.Close)
				for _, incoming := range []bool{false, true} {
					vms := []sqliteVM{{id: "vm1", name: "existing", clusterName: "cluster"}}
					owner := tc.owner
					if incoming {
						vms[0].name, owner = "incoming", "Payments"
						vms = append(vms, sqliteVM{id: "vm2", name: "new", clusterName: "cluster"})
					}
					center := "uuid"
					if incoming && tc.differentCenter {
						center = "other-uuid"
					}
					path := createTestSQLite(t, center, []sqliteCluster{{id: "c1", name: "cluster", datacenter: "dc"}}, vms)
					_, err = db.Exec(`ATTACH '` + escapeSQLString(path) + `' AS src (TYPE sqlite);
					ALTER TABLE src.VM ADD COLUMN Tags VARCHAR`)
					Expect(err).To(Succeed())
					tags, err := json.Marshal([]map[string]string{{"categoryID": "Owner", "name": owner}})
					Expect(err).To(Succeed())
					_, err = db.Exec(`UPDATE src.VM SET Tags = ? WHERE ID = 'vm1'`, string(tags))
					Expect(err).To(Succeed())
					if incoming {
						_, err = db.Exec(`UPDATE src.VM SET IsTemplate = ? WHERE ID = 'vm1'`, tc.template)
						Expect(err).To(Succeed())
						_, err = db.Exec(`UPDATE src.VM SET Tags = '[{"categoryID":"Owner","name":"Platform"}]' WHERE ID = 'vm2'`)
						Expect(err).To(Succeed())
					}
					_, err = db.Exec(`DETACH src`)
					Expect(err).To(Succeed())
					_, err = p.IngestSqlite(ctx, path)
					Expect(err).To(Succeed())
				}
				var name string
				Expect(p.db.QueryRowContext(ctx, `SELECT "VM" FROM vinfo WHERE "VM ID" = 'vm1'`).Scan(&name)).To(Succeed())
				Expect(name).To(Equal("existing"))
				expected := map[string][]string{}
				if tc.owner != "" {
					expected["Owner"] = []string{tc.owner}
				}
				if !tc.template && !tc.differentCenter {
					expected["Owner"] = append(expected["Owner"], "Payments")
				}
				Expect(storedMetadata(t, p.db, "vm1")).To(Equal(expected))
				Expect(storedMetadata(t, p.db, "vm2")).To(Equal(map[string][]string{"Owner": {"Platform"}}))
			})
		}
	})

	Context("RVTools metadata column layouts", func() {
		cases := []struct {
			name  string
			block []string
			want  bool
		}{
			{
				name:  "supported",
				block: []string{"Annotation", "Team's; \"name\" ", "Blank", "Datacenter"},
				want:  true,
			},

			{
				name:  "missing annotation",
				block: []string{"Team's; \"name\" ", "Datacenter"},
				want:  false,
			},

			{
				name:  "missing datacenter",
				block: []string{"Annotation", "Team's; \"name\" "},
				want:  false,
			},

			{
				name:  "reordered",
				block: []string{"Datacenter", "Team's; \"name\" ", "Annotation"},
				want:  false,
			},

			{
				name:  "duplicate boundary",
				block: []string{"Annotation", "Team's; \"name\" ", "Annotation", "Datacenter"},
				want:  false,
			},

			{
				name:  "duplicate datacenter",
				block: []string{"Annotation", "Team's; \"name\" ", "Datacenter", "Datacenter"},
				want:  false,
			},

			{
				name:  "duplicate metadata header",
				block: []string{"Annotation", "Team's; \"name\" ", "Team's; \"name\" ", "Datacenter"},
				want:  true,
			},

			{
				name:  "metadata column",
				block: []string{"Annotation", "metadata", "Datacenter"},
				want:  true,
			},

			{
				name:  "no metadata",
				block: []string{"Annotation", "Datacenter"},
				want:  false,
			},

			{
				name:  "older tags",
				block: []string{"Annotation", "Datacenter", "VI SDK API Version", "Team's; \"name\" ", "Blank", "VI SDK Server"},
				want:  true,
			},

			{
				name:  "older attributes and tags",
				block: []string{"Annotation", "metadata", "Datacenter", "VI SDK API Version", "Team's; \"name\" ", "VI SDK Server"},
				want:  true,
			},

			{
				name:  "older missing server",
				block: []string{"Annotation", "Datacenter", "VI SDK API Version", "Team's; \"name\" "},
				want:  false,
			},

			{
				name:  "older reordered",
				block: []string{"Annotation", "Datacenter", "VI SDK Server", "Team's; \"name\" ", "VI SDK API Version"},
				want:  false,
			},

			{
				name:  "older duplicate boundary",
				block: []string{"Annotation", "Datacenter", "VI SDK API Version", "Team's; \"name\" ", "VI SDK API Version", "VI SDK Server"},
				want:  false,
			},

			{
				name:  "older duplicate server",
				block: []string{"Annotation", "Datacenter", "VI SDK API Version", "Team's; \"name\" ", "VI SDK Server", "VI SDK Server"},
				want:  false,
			},

			{
				name:  "modern sdk columns",
				block: []string{"Annotation", "Team's; \"name\" ", "Datacenter", "VI SDK API Version", "VI SDK Server"},
				want:  true,
			},
		}

		for _, tc := range cases {
			It(tc.name, func() {
				t := GinkgoTB()

				p, _, cleanup := setupTestParser(t, nil)
				defer cleanup()
				headers := []string{"VM", "VM ID", "VI SDK UUID", "Host", "Cluster", "CPUs", "Memory", "Powerstate"}
				headers = append(headers, tc.block...)
				vms := []map[string]string{{"VM": "vm", "VM ID": "vm1", "VI SDK UUID": "uuid", "Host": "host", "Cluster": "cluster",
					"Datacenter": "dc", "Annotation": "free text", "Team's; \"name\" ": "  Payments; QA, [x]  ", "Blank": "  ", "metadata": "Original value"}}
				sheets := defaultStandardSheets(vms, []map[string]string{{"Host": "host", "Cluster": "cluster", "Datacenter": "dc"}})
				sheets[0].Headers = headers
				result, err := p.IngestRvTools(context.Background(), createTestExcel(t, sheets...))
				Expect(err).To(Succeed())
				Expect(result.HasErrors()).To(BeFalse())
				got, err := p.VMs(context.Background(), Filters{}, Options{})
				Expect(err).To(Succeed())
				Expect(got).To(HaveLen(1))
				if tc.want {
					switch tc.name {
					case "metadata column":
						Expect(got[0].Metadata).To(Equal(models.Metadata{"metadata": {"Original value"}}))
					case "older attributes and tags":
						Expect(got[0].Metadata).To(Equal(models.Metadata{"metadata": {"Original value"}, "Team's; \"name\" ": {"  Payments; QA, [x]  "}}))
					default:
						Expect(got[0].Metadata).To(Equal(models.Metadata{"Team's; \"name\" ": {"  Payments; QA, [x]  "}}))
					}
				} else {
					Expect(got[0].Metadata).To(BeEmpty())
				}
				Expect(got[0].Labels).To(BeEmpty())
			})
		}
	})

	Context("RVTools metadata for duplicate VM IDs", func() {
		cases := []struct {
			name           string
			firstTemplate  string
			secondTemplate string
			firstEmpty     bool
			secondEmpty    bool
			thirdDuplicate bool
		}{
			{name: "two eligible rows"},
			{name: "first row excluded", firstTemplate: "true"},
			{name: "second row excluded", secondTemplate: "true"},
			{name: "first metadata empty", firstEmpty: true},
			{name: "second metadata empty", secondEmpty: true},
			{name: "three duplicate rows", thirdDuplicate: true},
		}

		for _, tc := range cases {
			It(tc.name, func() {
				t := GinkgoTB()

				vms := []map[string]string{
					{"VM": "first", "VM ID": "vm1", "VI SDK UUID": "uuid", "Host": "host", "Cluster": "cluster",
						"Datacenter": "dc", "Template": tc.firstTemplate, "Owner": "Finance"},
					{"VM": "second", "VM ID": "vm1", "VI SDK UUID": "uuid", "Host": "host", "Cluster": "cluster",
						"Datacenter": "dc", "Template": tc.secondTemplate, "Owner": "Payments"},
					{"VM": "unique", "VM ID": "vm2", "VI SDK UUID": "uuid", "Host": "host", "Cluster": "cluster",
						"Datacenter": "dc", "Owner": "Platform"},
				}
				if tc.firstEmpty {
					vms[0]["Owner"] = ""
				}
				if tc.secondEmpty {
					vms[1]["Owner"] = ""
				}
				if tc.thirdDuplicate {
					vms = append(vms, map[string]string{"VM": "third", "VM ID": "vm1", "VI SDK UUID": "uuid", "Host": "host",
						"Cluster": "cluster", "Datacenter": "dc", "Owner": "Another"})
				}
				sheets := defaultStandardSheets(vms, []map[string]string{{"Host": "host", "Cluster": "cluster", "Datacenter": "dc"}})
				sheets[0].Headers = []string{"VM", "VM ID", "VI SDK UUID", "Host", "Cluster", "CPUs", "Memory", "Powerstate",
					"Template", "Annotation", "Owner", "Datacenter"}
				p, _, cleanup := setupTestParser(t, nil)
				defer cleanup()
				ctx := context.Background()
				result, err := p.IngestRvTools(ctx, createTestExcel(t, sheets...))
				Expect(err).To(Succeed())
				Expect(result.HasErrors()).To(BeFalse())
				got, err := p.VMs(ctx, Filters{}, Options{})
				Expect(err).To(Succeed())
				Expect(got).To(HaveLen(2))
				Expect([]string{got[0].ID, got[1].ID}).To(ConsistOf("vm1", "vm2"))

				for _, vm := range got {
					if vm.ID == "vm1" {
						Expect(vm.Metadata).To(BeEmpty(), "duplicate source IDs must not receive ambiguous metadata")
						if tc.firstTemplate == "true" {
							Expect(vm.Name).To(Equal("second"))
						} else if tc.secondTemplate == "true" {
							Expect(vm.Name).To(Equal("first"))
						}
					} else {
						Expect(vm.ID).To(Equal("vm2"))
						Expect(vm.Metadata).To(Equal(models.Metadata{"Owner": {"Platform"}}))
					}
				}
			})
		}
	})

	Context("RVTools metadata for existing VMs", func() {
		cases := []struct {
			template, owner string
			differentCenter bool
		}{
			{template: "false", owner: "Finance"},

			{template: "true", owner: "Finance"},

			{template: "false", owner: ""},

			{template: "false", owner: "Finance", differentCenter: true},
		}

		for _, tc := range cases {
			It(fmt.Sprintf("incoming template=%s, existing owner=%s, different vCenter=%t", tc.template, tc.owner, tc.differentCenter), func() {
				t := GinkgoTB()

				p, _, cleanup := setupTestParser(t, nil)
				defer cleanup()
				ctx := context.Background()
				vms := []map[string]string{{"VM": "existing", "VM ID": "vm1", "VI SDK UUID": "uuid", "Host": "host",
					"Cluster": "cluster", "Datacenter": "dc", "Owner": tc.owner}}
				sheets := defaultStandardSheets(vms, []map[string]string{{"Host": "host", "Cluster": "cluster", "Datacenter": "dc"}})
				sheets[0].Headers = []string{"VM", "VM ID", "VI SDK UUID", "Host", "Cluster", "CPUs", "Memory", "Powerstate",
					"Template", "Annotation", "Owner", "Datacenter"}
				_, err := p.IngestRvTools(ctx, createTestExcel(t, sheets...))
				Expect(err).To(Succeed())
				vms[0]["VM"], vms[0]["Owner"], vms[0]["Template"] = "incoming", "Payments", tc.template
				center := "uuid"
				if tc.differentCenter {
					center = "other-uuid"
				}
				vms[0]["VI SDK UUID"] = center
				sheets[0].Rows = append(vms, map[string]string{"VM": "new", "VM ID": "vm2", "VI SDK UUID": center, "Host": "host",
					"Cluster": "cluster", "Datacenter": "dc", "Owner": "Platform"})
				_, err = p.IngestRvTools(ctx, createTestExcel(t, sheets...))
				Expect(err).To(Succeed())
				var name string
				Expect(p.db.QueryRowContext(ctx, `SELECT "VM" FROM vinfo WHERE "VM ID" = 'vm1'`).Scan(&name)).To(Succeed())
				Expect(name).To(Equal("existing"))
				expected := map[string][]string{}
				if tc.owner != "" {
					expected["Owner"] = []string{tc.owner}
				}
				if tc.template == "false" && !tc.differentCenter {
					expected["Owner"] = append(expected["Owner"], "Payments")
				}
				Expect(storedMetadata(t, p.db, "vm1")).To(Equal(expected))
				Expect(storedMetadata(t, p.db, "vm2")).To(Equal(map[string][]string{"Owner": {"Platform"}}))
			})
		}
	})

	It("stops later writes after malformed RVTools metadata", func() {
		t := GinkgoTB()

		p, db, cleanup := setupTestParser(t, nil)
		defer cleanup()
		ctx := context.Background()
		err := p.executeStatements(ctx, `INSERT INTO vinfo ("VM ID", "VM") VALUES ('vm1', 'first');
    WITH metadata AS (
        SELECT key AS vm_id, from_json(value, '"MAP(VARCHAR, VARCHAR[])"') AS entries FROM json_each(?)
    )
    INSERT INTO vm_metadata SELECT metadata.vm_id, entry.key, unnest(entry.value)
    FROM metadata, unnest(map_entries(metadata.entries)) m(entry);
    INSERT INTO vinfo ("VM ID", "VM") VALUES ('vm2', 'later');`, "invalid JSON")
		Expect(err).To(MatchError(ContainSubstring("Malformed JSON")))
		var count int
		Expect(db.QueryRowContext(ctx, `SELECT count(*) FROM vinfo`).Scan(&count)).To(Succeed())
		Expect(count).To(Equal(1), "metadata failure must stop later statements")
		Expect(db.QueryRowContext(ctx, `SELECT count(*) FROM vm_metadata`).Scan(&count)).To(Succeed())
		Expect(count).To(BeZero())
	})

	It("handles missing workbooks, missing sheets and empty headers", func() {
		t := GinkgoTB()

		ctx := context.Background()
		_, err := readRVToolsMetadata(ctx, t.TempDir()+"/missing.xlsx")
		Expect(err).To(MatchError(ContainSubstring("opening RVTools workbook")))

		missingSheet := createTestExcel(t, NewExcelSheet("vHost", []string{"Host"}, nil))
		_, err = readRVToolsMetadata(ctx, missingSheet)
		Expect(err).To(MatchError(ContainSubstring("reading vInfo headers")))

		emptySheet := createTestExcel(t, NewExcelSheet("vInfo", nil, nil))
		metadata, err := readRVToolsMetadata(ctx, emptySheet)
		Expect(err).To(Succeed())
		Expect(metadata).To(BeEmpty())

		query, err := NewBuilder().IngestRvtoolsQuery("not-opened.xlsx", false)
		Expect(err).To(Succeed())
		Expect(query).NotTo(ContainSubstring("WITH metadata AS ("))
	})

	It("skips formatted blank cells while retaining genuine metadata values", func() {
		t := GinkgoTB()

		// Keep genuine "VM" values and skip formatted blanks.
		vms := []map[string]string{
			{"VM": "vm", "VM ID": "vm1", "VI SDK UUID": "uuid", "Host": "host", "Cluster": "cluster", "Datacenter": "dc",
				"Annotation": "notes", "Blank": "replace-with-formatted-blank", "Real": "VM", "Text": "  Finance's; [QA]  "},
			{"VM": "empty", "VM ID": "vm2", "VI SDK UUID": "uuid", "Host": "host", "Cluster": "cluster", "Datacenter": "dc"},
			{"VM": "template", "VM ID": "vm3", "VI SDK UUID": "uuid", "Host": "host", "Cluster": "cluster", "Datacenter": "dc",
				"Template": "true", "Real": "excluded template value"},
		}
		sheets := defaultStandardSheets(vms, []map[string]string{{"Host": "host", "Cluster": "cluster", "Datacenter": "dc"}})
		sheets[0].Headers = []string{"VM", "VM ID", "VI SDK UUID", "Host", "Cluster", "CPUs", "Memory", "Powerstate", "Template",
			"Annotation", "Blank", "Real", "Text", "Datacenter"}
		path := createTestExcel(t, sheets...)
		source, err := zip.OpenReader(path)
		Expect(err).To(Succeed())
		DeferCleanup(source.Close)
		var buffer bytes.Buffer
		writer := zip.NewWriter(&buffer)
		replaced := false
		for _, file := range source.File {
			reader, err := file.Open()
			Expect(err).To(Succeed())
			data, err := io.ReadAll(reader)
			Expect(err).To(Succeed())
			Expect(reader.Close()).To(Succeed())
			if file.Name == "xl/worksheets/sheet2.xml" {
				// RVTools emits shared-string cells without a value for formatted blanks.
				pattern := regexp.MustCompile(`<c\b[^>]*\br="K2"[^>]*>.*?</c>`)
				Expect(pattern.Match(data)).To(BeTrue())
				data = pattern.ReplaceAll(data, []byte(`<c r="K2" s="0" t="s"/>`))
				replaced = true
			}
			entry, err := writer.CreateHeader(&file.FileHeader)
			Expect(err).To(Succeed())
			_, err = entry.Write(data)
			Expect(err).To(Succeed())
		}
		Expect(replaced).To(BeTrue())
		Expect(writer.Close()).To(Succeed())
		path = filepath.Join(t.TempDir(), "formatted-blanks.xlsx")
		Expect(os.WriteFile(path, buffer.Bytes(), 0600)).To(Succeed())

		p, _, cleanup := setupTestParser(t, nil)
		defer cleanup()
		result, err := p.IngestRvTools(context.Background(), path)
		Expect(err).To(Succeed())
		Expect(result.HasErrors()).To(BeFalse())
		got, err := p.VMs(context.Background(), Filters{}, Options{})
		Expect(err).To(Succeed())
		Expect(got).To(HaveLen(2), "templates must remain excluded")
		Expect([]string{got[0].ID, got[1].ID}).To(ConsistOf("vm1", "vm2"))

		for _, vm := range got {
			if vm.ID == "vm1" {
				Expect(vm.Metadata).To(Equal(models.Metadata{"Real": {"VM"}, "Text": {"  Finance's; [QA]  "}}))
			} else {
				Expect(vm.Metadata).To(BeEmpty())
			}
		}
	})

	It("reads older inventories without creating a metadata table", func() {
		db, err := sql.Open("duckdb", "")
		Expect(err).To(Succeed())
		DeferCleanup(db.Close)
		p := New(db, nil)
		schema, err := p.builder.CreateSchemaQuery()
		Expect(err).To(Succeed())
		schema = regexp.MustCompile(`(?s)CREATE TABLE IF NOT EXISTS vm_metadata \(.*?\);`).ReplaceAllString(schema, "")
		_, err = db.Exec(schema)
		Expect(err).To(Succeed())
		_, err = db.Exec(`INSERT INTO vinfo ("VM ID", "VM") VALUES ('vm1', 'vm')`)
		Expect(err).To(Succeed())
		vms, err := p.VMs(context.Background(), Filters{}, Options{})
		Expect(err).To(Succeed())
		Expect(vms[0].Metadata).To(BeEmpty())
		var count int
		Expect(db.QueryRow(`SELECT count(*) FROM information_schema.tables WHERE table_name='vm_metadata'`).Scan(&count)).To(Succeed())
		Expect(count).To(BeZero())
	})

	It("stops later statements when metadata persistence fails", func() {
		t := GinkgoTB()

		p, db, cleanup := setupTestParser(t, nil)
		defer cleanup()
		err := p.executeStatements(context.Background(), `INSERT INTO vm_metadata (vm_id, key, value) VALUES ('missing', 'Owner', 'Finance');
        INSERT INTO vinfo ("VM ID", "VM") VALUES ('later', 'later');`)
		Expect(err).To(HaveOccurred())
		var count int
		Expect(db.QueryRow(`SELECT count(*) FROM vinfo`).Scan(&count)).To(Succeed())
		Expect(count).To(BeZero(), "failed metadata persistence must stop later statements")
	})

	It("clears metadata and storage details before parser reuse", func() {
		t := GinkgoTB()

		p, _, cleanup := setupTestParser(t, nil)
		defer cleanup()
		ctx := context.Background()
		_, err := p.db.ExecContext(ctx, `INSERT INTO vinfo ("VM ID", "VM") VALUES ('vm1', 'vm');
        INSERT INTO vm_metadata (vm_id, key, value) VALUES ('vm1', 'Owner', 'Alice');
        INSERT INTO vhostscsi ("Host ID", "Canonical Name", "LUN Vendor", "LUN Model")
        VALUES ('host-1', 'naa.shared', 'OLD', 'old-model')`)
		Expect(err).To(Succeed())
		Expect(p.Cleanup()).To(Succeed())
		Expect(storedMetadata(t, p.db, "vm1")).To(BeEmpty())
		vms, err := p.VMs(ctx, Filters{}, Options{})
		Expect(err).To(Succeed())
		Expect(vms).To(BeEmpty())
		_, err = p.db.ExecContext(ctx, `INSERT INTO vhost ("Object ID", "Host") VALUES ('host-1', 'esxi1');
        INSERT INTO vdatastore ("Name", "Hosts", "Backing Devices") VALUES ('new-datastore', 'esxi1', '["naa.shared"]')`)
		Expect(err).To(Succeed())
		datastores, err := p.Datastores(ctx, Filters{}, Options{})
		Expect(err).To(Succeed())
		Expect(datastores).To(HaveLen(1))
		Expect(datastores[0].Vendor).To(Equal("N/A"), "previous LUN metadata must not survive parser reuse")
		Expect(datastores[0].Model).To(Equal("N/A"))
	})
})

type cancelVMWriteDB struct {
	*sql.DB
	cancel context.CancelFunc
}

func (db *cancelVMWriteDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if db.cancel != nil && strings.HasPrefix(query, "INSERT INTO vinfo (") {
		db.cancel()
		db.cancel = nil
	}
	return db.DB.ExecContext(ctx, query, args...)
}

func storedMetadata(t testing.TB, db store.QueryInterceptor, id string) map[string][]string {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), `SELECT key, value FROM vm_metadata WHERE vm_id = ? ORDER BY key, value`, id)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	values := make(map[string][]string)
	for rows.Next() {
		var key, value string
		require.NoError(t, rows.Scan(&key, &value))
		values[key] = append(values[key], value)
	}
	require.NoError(t, rows.Err())
	return values
}

func TestRVToolsValidationErrorDetails(t *testing.T) {
	require.NoError(t, (ValidationResult{}).Error())
	require.NoError(t, (ValidationResult{Warnings: []ValidationIssue{{Message: "warning"}}}).Error())
	result := ValidationResult{Errors: []ValidationIssue{
		{Code: CodeMissingVMID, Message: "First issue: missing VM ID"},
		{Code: CodeColumnValidationFailed, Message: "SELECT private_column FROM /private/database"},
		{Code: CodeMissingVMName, Message: "Second issue: missing name"},
	}}
	err := fmt.Errorf("ingesting: %w", result.Error())
	var validation *SchemaValidationError
	require.False(t, errors.As(err, &validation), "technical issues must use the caller's fallback")
	require.Contains(t, err.Error(), "private_column")
	require.Contains(t, err.Error(), "First issue: missing VM ID")
	require.Contains(t, err.Error(), "Second issue: missing name")
	require.Len(t, result.Errors, 3)
}

func TestRVToolsWorkbookErrorMessages(t *testing.T) {
	for _, tc := range []struct {
		name, sheet, content string
		truncate             bool
	}{
		{name: "JSON", content: "{}"},
		{name: "empty"},
		{name: "missing vInfo", sheet: "vHost"},
		{name: "truncated", sheet: "vInfo", truncate: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "private.xlsx")
			if tc.sheet == "" {
				require.NoError(t, os.WriteFile(path, []byte(tc.content), 0600))
			} else {
				path = createTestExcel(t, NewExcelSheet(tc.sheet, []string{"VM ID", "VM", "Cluster", "VI SDK UUID"}, nil))
				if tc.truncate {
					data, err := os.ReadFile(path)
					require.NoError(t, err)
					require.NoError(t, os.Truncate(path, int64(len(data)/2)))
				}
			}
			_, err := readRVToolsMetadata(context.Background(), path)
			require.Error(t, err)
			if tc.sheet == "" || tc.truncate {
				require.ErrorIs(t, err, zip.ErrFormat)
			} else {
				var missing excelize.ErrSheetNotExist
				require.ErrorAs(t, err, &missing)
				require.Equal(t, "vInfo", missing.SheetName)
			}
		})
	}
	_, err := readRVToolsMetadata(context.Background(), "/does/not/exist/private.xlsx")
	require.ErrorIs(t, err, os.ErrNotExist)
}
