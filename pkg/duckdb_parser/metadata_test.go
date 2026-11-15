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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"github.com/kubev2v/migration-planner/pkg/duckdb_parser/models"
	"github.com/kubev2v/migration-planner/pkg/store"
)

var _ = Describe("VM metadata", func() {
	var (
		ctx context.Context
		p   *Parser
		db  *sql.DB
	)

	BeforeEach(func() {
		ctx = context.Background()

		var cleanup func()
		p, db, cleanup = setupTestParser(GinkgoTB(), nil)
		DeferCleanup(cleanup)
	})

	Context("SQLite tags and custom attributes", func() {
		var (
			path      string
			fixtureDB *sql.DB
		)

		BeforeEach(func() {
			path = createTestSQLite(GinkgoTB(), "uuid", []sqliteCluster{{
				id:         "c1",
				name:       "cluster",
				datacenter: "dc",
			}},
				[]sqliteVM{{
					id:          "vm1",
					name:        "vm",
					clusterName: "cluster",
				}})

			var err error
			fixtureDB, err = sql.Open("duckdb", "")
			Expect(err).To(Succeed())
			DeferCleanup(fixtureDB.Close)

			_, err = fixtureDB.Exec(`ATTACH '` + escapeSQLString(path) + `' AS src (TYPE sqlite);
				ALTER TABLE src.VM ADD COLUMN Tags VARCHAR;
				ALTER TABLE src.VM ADD COLUMN CustomDef VARCHAR;
				ALTER TABLE src.VM ADD COLUMN CustomValues VARCHAR`)
			Expect(err).To(Succeed())

			tags := `[
				{"categoryID":"cat1","name":"Payments; QA"},
				{"categoryID":"cat1","name":"Production"},
				{"categoryID":"cat1","name":"Payments; QA"},
				{"categoryID":"cat2","name":"QA"}
			]`
			customDefinitions := `[
				{"key":1,"name":"cat1"},
				{"key":2,"name":"Owner's; \"name\""},
				{"key":3,"name":"Empty"}
			]`
			customValues := `[
				{"key":1,"value":"  Finance & IT  "},
				{"key":2,"value":"HR"},
				{"key":3,"value":" "},
				{"key":99,"value":"unmatched"}
			]`

			_, err = fixtureDB.Exec(
				`UPDATE src.VM SET Tags = ?, CustomDef = ?, CustomValues = ?`,
				tags, customDefinitions, customValues,
			)
			Expect(err).To(Succeed())

			_, err = fixtureDB.Exec(`DETACH src`)
			Expect(err).To(Succeed())
		})

		It("imports tags and custom attributes", func() {
			_, err := p.db.ExecContext(ctx, `INSERT INTO tag_categories VALUES (?, ?)`, "cat1", "Owner's; \"name\"")
			Expect(err).To(Succeed())

			result, err := p.IngestSqlite(ctx, path)
			Expect(err).To(Succeed())
			Expect(result.HasErrors()).To(BeFalse())

			var temporaryTables int
			err = p.db.QueryRowContext(ctx, `
				SELECT count(*)
				FROM information_schema.tables
				WHERE table_catalog = 'temp' AND table_name = 'tag_categories'
			`).Scan(&temporaryTables)
			Expect(err).To(Succeed())
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

			err = p.db.QueryRowContext(ctx, `
				SELECT count(*)
				FROM information_schema.columns
				WHERE table_name = 'vinfo' AND column_name = 'metadata'
			`).Scan(&count)
			Expect(err).To(Succeed())
			Expect(count).To(BeZero())
			Expect(vms[0].Labels).To(BeEmpty())
		})

		DescribeTable("omits unresolved tags without losing custom attributes",
			func(categoryNames []string) {
				for _, categoryName := range categoryNames {
					_, err := p.db.ExecContext(ctx, `INSERT INTO tag_categories VALUES ('cat1', ?)`, categoryName)
					Expect(err).To(Succeed())
				}

				result, err := p.IngestSqlite(ctx, path)
				Expect(err).To(Succeed())
				Expect(result.HasErrors()).To(BeFalse())

				vms, err := p.VMs(ctx, Filters{}, Options{})
				Expect(err).To(Succeed())
				Expect(vms).To(HaveLen(1))
				Expect(vms[0].Metadata).To(Equal(models.Metadata{
					"cat1":              {"  Finance & IT  "},
					"Owner's; \"name\"": {"HR"},
				}), "unresolved tags must be omitted without losing custom attributes")
			},
			Entry("empty category name", []string{""}),
			Entry("blank category name", []string{" "}),
			Entry("missing category", []string(nil)),
		)

		It("keeps aggregate inventory unchanged by metadata", func() {
			_, err := p.db.ExecContext(ctx, `INSERT INTO tag_categories VALUES (?, ?)`, "cat1", "Owner's; \"name\"")
			Expect(err).To(Succeed())

			result, err := p.IngestSqlite(ctx, path)
			Expect(err).To(Succeed())
			Expect(result.HasErrors()).To(BeFalse())

			withMetadata, err := p.BuildInventory(ctx, nil)
			Expect(err).To(Succeed())

			_, err = fixtureDB.Exec(`ATTACH '` + escapeSQLString(path) + `' AS src (TYPE sqlite);
				UPDATE src.VM SET Tags = '[]', CustomValues = '[]'; DETACH src`)
			Expect(err).To(Succeed())

			fresh, _, closeFresh := setupTestParser(GinkgoTB(), nil)
			DeferCleanup(closeFresh)

			_, err = fresh.IngestSqlite(ctx, path)
			Expect(err).To(Succeed())

			vms, err := fresh.VMs(ctx, Filters{}, Options{})
			Expect(err).To(Succeed())
			Expect(vms).To(HaveLen(1))
			Expect(vms[0].Metadata).To(BeEmpty())

			withoutMetadata, err := fresh.BuildInventory(ctx, nil)
			Expect(err).To(Succeed())

			withMetadata.CreatedAt, withoutMetadata.CreatedAt = nil, nil
			Expect(withoutMetadata).To(Equal(withMetadata), "metadata must not change aggregate inventory")
		})
	})

	Context("after malformed SQLite metadata", func() {
		BeforeEach(func() {
			path := createTestSQLite(GinkgoTB(), "uuid", []sqliteCluster{{
				id:         "c1",
				name:       "cluster",
				datacenter: "dc",
			}},
				[]sqliteVM{{
					id:          "vm1",
					name:        "vm",
					clusterName: "cluster",
				}})
			db, err := sql.Open("duckdb", "")
			Expect(err).To(Succeed())
			DeferCleanup(db.Close)

			_, err = db.Exec(`ATTACH '` + escapeSQLString(path) + `' AS src (TYPE sqlite);
				ALTER TABLE src.VM ADD COLUMN Tags VARCHAR;
				UPDATE src.VM SET Tags = 'invalid JSON'; DETACH src`)
			Expect(err).To(Succeed())

			_, err = p.db.ExecContext(context.Background(), `INSERT INTO tag_categories VALUES ('cat1', 'Owner')`)
			Expect(err).To(Succeed())

			_, err = p.IngestSqlite(context.Background(), path)
			Expect(err).To(MatchError(ContainSubstring("Malformed JSON")))
		})

		It("stops later inventory writes", func() {
			var count int
			Expect(db.QueryRowContext(ctx, `SELECT count(*) FROM vinfo`).Scan(&count)).To(Succeed())
			Expect(count).To(Equal(1), "VM insert precedes metadata, but failed ingestion must not be published")

			Expect(db.QueryRowContext(ctx, `SELECT count(*) FROM vcpu`).Scan(&count)).To(Succeed())
			Expect(count).To(BeZero(), "metadata failure must stop later inventory writes")
		})

		It("detaches its SQLite source", func() {
			var count int
			Expect(db.QueryRowContext(ctx, `SELECT count(*) FROM duckdb_databases() WHERE database_name = 'src'`).Scan(&count)).To(Succeed())
			Expect(count).To(BeZero())
		})

		It("preserves category names for retry", func() {
			var count int
			Expect(db.QueryRowContext(ctx, `SELECT count(*) FROM tag_categories`).Scan(&count)).To(Succeed())
			Expect(count).To(Equal(1))
		})

		It("permits a clean retry", func() {
			validPath := createTestSQLite(GinkgoTB(), "uuid", []sqliteCluster{{
				id:         "c1",
				name:       "cluster",
				datacenter: "dc",
			}},
				[]sqliteVM{{
					id:          "vm2",
					name:        "replacement",
					clusterName: "cluster",
				}})
			_, err := p.IngestSqlite(context.Background(), validPath)
			Expect(err).To(Succeed())

			var name string
			Expect(p.db.QueryRowContext(context.Background(), `SELECT "VM" FROM vinfo WHERE "VM ID" = 'vm2'`).Scan(&name)).To(Succeed())
			Expect(name).To(Equal("replacement"))
		})
	})

	It("preserves category names when retrying a cancelled SQLite import", func() {
		path := createTestSQLite(GinkgoTB(), "uuid", []sqliteCluster{{
			id:         "c1",
			name:       "cluster",
			datacenter: "dc",
		}},
			[]sqliteVM{{
				id:          "vm1",
				name:        "vm",
				clusterName: "cluster",
			}})
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)

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
		sheets := defaultStandardSheets([]map[string]string{{
			"VM":          "vm",
			"VM ID":       "vm1",
			"VI SDK UUID": "uuid",
			"Host":        "host",
			"Cluster":     "cluster",
			"Datacenter":  "dc",
			"Owner":       "Finance",
		}},
			[]map[string]string{{
				"Host":       "host",
				"Cluster":    "cluster",
				"Datacenter": "dc",
			}})
		sheets[0].Headers = []string{"VM", "VM ID", "VI SDK UUID", "Host", "Cluster", "CPUs", "Memory", "Powerstate", "Template",
			"Annotation", "Owner", "Datacenter"}
		path := createTestExcel(GinkgoTB(), sheets...)
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)

		p.db = &cancelVMWriteDB{DB: db, cancel: cancel}
		_, err := p.IngestRvTools(ctx, path)
		Expect(err).To(MatchError(context.Canceled))

		Expect(p.Cleanup()).To(Succeed())

		_, err = p.IngestRvTools(context.Background(), path)
		Expect(err).To(Succeed())
		Expect(storedMetadata(GinkgoTB(), db, "vm1")).To(Equal(map[string][]string{"Owner": {"Finance"}}))
	})

	DescribeTable("SQLite metadata for existing VMs",
		func(incomingTemplate bool, existingOwner, incomingCenter string, expected map[string][]string) {
			_, err := p.db.ExecContext(ctx, `INSERT INTO tag_categories VALUES ('Owner', 'Owner')`)
			Expect(err).To(Succeed())

			fixtureDB, err := sql.Open("duckdb", "")
			Expect(err).To(Succeed())
			DeferCleanup(fixtureDB.Close)

			type metadataVM struct {
				id       string
				name     string
				owner    string
				template bool
			}

			imports := []struct {
				center string
				vms    []metadataVM
			}{
				{
					center: "uuid",
					vms: []metadataVM{{
						id:    "vm1",
						name:  "existing",
						owner: existingOwner,
					}},
				},
				{
					center: incomingCenter,
					vms: []metadataVM{
						{
							id:       "vm1",
							name:     "incoming",
							owner:    "Payments",
							template: incomingTemplate,
						},
						{
							id:    "vm2",
							name:  "new",
							owner: "Platform",
						},
					},
				},
			}

			for _, inventory := range imports {
				vms := make([]sqliteVM, 0, len(inventory.vms))

				for _, vm := range inventory.vms {
					vms = append(vms, sqliteVM{
						id:          vm.id,
						name:        vm.name,
						clusterName: "cluster",
					})
				}

				path := createTestSQLite(GinkgoTB(), inventory.center, []sqliteCluster{{
					id:         "c1",
					name:       "cluster",
					datacenter: "dc",
				}}, vms)

				_, err = fixtureDB.Exec(`ATTACH '` + escapeSQLString(path) + `' AS src (TYPE sqlite);
					ALTER TABLE src.VM ADD COLUMN Tags VARCHAR`)
				Expect(err).To(Succeed())

				for _, vm := range inventory.vms {
					tags, err := json.Marshal([]map[string]string{{"categoryID": "Owner", "name": vm.owner}})
					Expect(err).To(Succeed())

					_, err = fixtureDB.Exec(`UPDATE src.VM SET Tags = ?, IsTemplate = ? WHERE ID = ?`,
						string(tags), vm.template, vm.id)
					Expect(err).To(Succeed())
				}

				_, err = fixtureDB.Exec(`DETACH src`)
				Expect(err).To(Succeed())

				_, err = p.IngestSqlite(ctx, path)
				Expect(err).To(Succeed())
			}

			var name string
			Expect(p.db.QueryRowContext(ctx, `SELECT "VM" FROM vinfo WHERE "VM ID" = 'vm1'`).Scan(&name)).To(Succeed())
			Expect(name).To(Equal("existing"))
			Expect(storedMetadata(GinkgoTB(), p.db, "vm1")).To(Equal(expected))
			Expect(storedMetadata(GinkgoTB(), p.db, "vm2")).To(Equal(map[string][]string{"Owner": {"Platform"}}))
		},
		Entry("merges metadata for the same vCenter",
			false, "Finance", "uuid", map[string][]string{"Owner": {"Finance", "Payments"}},
		),

		Entry("ignores incoming template metadata",
			true, "Finance", "uuid", map[string][]string{"Owner": {"Finance"}},
		),

		Entry("adds metadata to an existing VM without an owner",
			false, "", "uuid", map[string][]string{"Owner": {"Payments"}},
		),

		Entry("keeps metadata from another vCenter separate",
			false, "Finance", "other-uuid", map[string][]string{"Owner": {"Finance"}},
		),
	)

	DescribeTable("RVTools metadata column layouts",
		func(block []string, expected models.Metadata) {
			headers := []string{"VM", "VM ID", "VI SDK UUID", "Host", "Cluster", "CPUs", "Memory", "Powerstate"}
			headers = append(headers, block...)
			vms := []map[string]string{{
				"VM":                "vm",
				"VM ID":             "vm1",
				"VI SDK UUID":       "uuid",
				"Host":              "host",
				"Cluster":           "cluster",
				"Datacenter":        "dc",
				"Annotation":        "free text",
				"Team's; \"name\" ": "  Payments; QA, [x]  ",
				"Blank":             "  ",
				"metadata":          "Original value",
			}}
			sheets := defaultStandardSheets(vms, []map[string]string{{
				"Host":       "host",
				"Cluster":    "cluster",
				"Datacenter": "dc",
			}})
			sheets[0].Headers = headers
			result, err := p.IngestRvTools(context.Background(), createTestExcel(GinkgoTB(), sheets...))
			Expect(err).To(Succeed())
			Expect(result.HasErrors()).To(BeFalse())

			got, err := p.VMs(context.Background(), Filters{}, Options{})
			Expect(err).To(Succeed())
			Expect(got).To(HaveLen(1))
			Expect(got[0].Metadata).To(Equal(expected))
			Expect(got[0].Labels).To(BeEmpty())
		},
		Entry("supported",
			[]string{"Annotation", "Team's; \"name\" ", "Blank", "Datacenter"},
			models.Metadata{"Team's; \"name\" ": {"  Payments; QA, [x]  "}},
		),

		Entry("missing annotation",
			[]string{"Team's; \"name\" ", "Datacenter"},
			models.Metadata{},
		),

		Entry("missing datacenter",
			[]string{"Annotation", "Team's; \"name\" "},
			models.Metadata{},
		),

		Entry("reordered",
			[]string{"Datacenter", "Team's; \"name\" ", "Annotation"},
			models.Metadata{},
		),

		Entry("duplicate boundary",
			[]string{"Annotation", "Team's; \"name\" ", "Annotation", "Datacenter"},
			models.Metadata{},
		),

		Entry("duplicate datacenter",
			[]string{"Annotation", "Team's; \"name\" ", "Datacenter", "Datacenter"},
			models.Metadata{},
		),

		Entry("duplicate metadata header",
			[]string{"Annotation", "Team's; \"name\" ", "Team's; \"name\" ", "Datacenter"},
			models.Metadata{"Team's; \"name\" ": {"  Payments; QA, [x]  "}},
		),

		Entry("metadata column",
			[]string{"Annotation", "metadata", "Datacenter"},
			models.Metadata{"metadata": {"Original value"}},
		),

		Entry("no metadata",
			[]string{"Annotation", "Datacenter"},
			models.Metadata{},
		),

		Entry("older tags",
			[]string{"Annotation", "Datacenter", "VI SDK API Version", "Team's; \"name\" ", "Blank", "VI SDK Server"},
			models.Metadata{"Team's; \"name\" ": {"  Payments; QA, [x]  "}},
		),

		Entry("older attributes and tags",
			[]string{"Annotation", "metadata", "Datacenter", "VI SDK API Version", "Team's; \"name\" ", "VI SDK Server"},
			models.Metadata{
				"metadata":          {"Original value"},
				"Team's; \"name\" ": {"  Payments; QA, [x]  "},
			},
		),

		Entry("older missing server",
			[]string{"Annotation", "Datacenter", "VI SDK API Version", "Team's; \"name\" "},
			models.Metadata{},
		),

		Entry("older reordered",
			[]string{"Annotation", "Datacenter", "VI SDK Server", "Team's; \"name\" ", "VI SDK API Version"},
			models.Metadata{},
		),

		Entry("older duplicate boundary",
			[]string{"Annotation", "Datacenter", "VI SDK API Version", "Team's; \"name\" ", "VI SDK API Version", "VI SDK Server"},
			models.Metadata{},
		),

		Entry("older duplicate server",
			[]string{"Annotation", "Datacenter", "VI SDK API Version", "Team's; \"name\" ", "VI SDK Server", "VI SDK Server"},
			models.Metadata{},
		),

		Entry("modern sdk columns",
			[]string{"Annotation", "Team's; \"name\" ", "Datacenter", "VI SDK API Version", "VI SDK Server"},
			models.Metadata{"Team's; \"name\" ": {"  Payments; QA, [x]  "}},
		),
	)

	type duplicateVMRow struct {
		template string
		owner    string
	}

	DescribeTable("RVTools metadata for duplicate VM IDs",
		func(first, second duplicateVMRow, additionalOwner string) {
			vms := []map[string]string{
				{
					"VM":          "first",
					"VM ID":       "vm1",
					"VI SDK UUID": "uuid",
					"Host":        "host",
					"Cluster":     "cluster",
					"Datacenter":  "dc",
					"Template":    first.template,
					"Owner":       first.owner,
				},
				{
					"VM":          "second",
					"VM ID":       "vm1",
					"VI SDK UUID": "uuid",
					"Host":        "host",
					"Cluster":     "cluster",
					"Datacenter":  "dc",
					"Template":    second.template,
					"Owner":       second.owner,
				},
				{
					"VM":          "unique",
					"VM ID":       "vm2",
					"VI SDK UUID": "uuid",
					"Host":        "host",
					"Cluster":     "cluster",
					"Datacenter":  "dc",
					"Owner":       "Platform",
				},
			}
			if additionalOwner != "" {
				vms = append(vms, map[string]string{
					"VM":          "third",
					"VM ID":       "vm1",
					"VI SDK UUID": "uuid",
					"Host":        "host",
					"Cluster":     "cluster",
					"Datacenter":  "dc",
					"Owner":       additionalOwner,
				})
			}
			sheets := defaultStandardSheets(vms, []map[string]string{{
				"Host":       "host",
				"Cluster":    "cluster",
				"Datacenter": "dc",
			}})
			sheets[0].Headers = []string{"VM", "VM ID", "VI SDK UUID", "Host", "Cluster", "CPUs", "Memory", "Powerstate",
				"Template", "Annotation", "Owner", "Datacenter"}

			result, err := p.IngestRvTools(ctx, createTestExcel(GinkgoTB(), sheets...))
			Expect(err).To(Succeed())
			Expect(result.HasErrors()).To(BeFalse())

			got, err := p.VMs(ctx, Filters{}, Options{})
			Expect(err).To(Succeed())
			Expect(got).To(HaveLen(2))
			Expect([]string{got[0].ID, got[1].ID}).To(ConsistOf("vm1", "vm2"))

			for _, vm := range got {
				if vm.ID == "vm1" {
					Expect(vm.Metadata).To(BeEmpty(), "duplicate source IDs must not receive ambiguous metadata")
					if first.template == "true" {
						Expect(vm.Name).To(Equal("second"))
					} else if second.template == "true" {
						Expect(vm.Name).To(Equal("first"))
					}
				} else {
					Expect(vm.ID).To(Equal("vm2"))
					Expect(vm.Metadata).To(Equal(models.Metadata{"Owner": {"Platform"}}))
				}
			}
		},
		Entry("two eligible rows",
			duplicateVMRow{owner: "Finance"},
			duplicateVMRow{owner: "Payments"},
			"",
		),

		Entry("first row excluded",
			duplicateVMRow{template: "true", owner: "Finance"},
			duplicateVMRow{owner: "Payments"},
			"",
		),

		Entry("second row excluded",
			duplicateVMRow{owner: "Finance"},
			duplicateVMRow{template: "true", owner: "Payments"},
			"",
		),

		Entry("first metadata empty",
			duplicateVMRow{},
			duplicateVMRow{owner: "Payments"},
			"",
		),

		Entry("second metadata empty",
			duplicateVMRow{owner: "Finance"},
			duplicateVMRow{},
			"",
		),

		Entry("three duplicate rows",
			duplicateVMRow{owner: "Finance"},
			duplicateVMRow{owner: "Payments"},
			"Another",
		),
	)

	DescribeTable("RVTools metadata for existing VMs",
		func(incomingTemplate, existingOwner, incomingCenter string, expected map[string][]string) {
			vms := []map[string]string{{
				"VM":          "existing",
				"VM ID":       "vm1",
				"VI SDK UUID": "uuid",
				"Host":        "host",
				"Cluster":     "cluster",
				"Datacenter":  "dc",
				"Owner":       existingOwner,
			}}
			sheets := defaultStandardSheets(vms, []map[string]string{{
				"Host":       "host",
				"Cluster":    "cluster",
				"Datacenter": "dc",
			}})
			sheets[0].Headers = []string{"VM", "VM ID", "VI SDK UUID", "Host", "Cluster", "CPUs", "Memory", "Powerstate",
				"Template", "Annotation", "Owner", "Datacenter"}

			_, err := p.IngestRvTools(ctx, createTestExcel(GinkgoTB(), sheets...))
			Expect(err).To(Succeed())

			vms[0]["VM"], vms[0]["Owner"], vms[0]["Template"] = "incoming", "Payments", incomingTemplate
			vms[0]["VI SDK UUID"] = incomingCenter
			sheets[0].Rows = append(vms, map[string]string{
				"VM":          "new",
				"VM ID":       "vm2",
				"VI SDK UUID": incomingCenter,
				"Host":        "host",
				"Cluster":     "cluster",
				"Datacenter":  "dc",
				"Owner":       "Platform",
			})

			_, err = p.IngestRvTools(ctx, createTestExcel(GinkgoTB(), sheets...))
			Expect(err).To(Succeed())

			var name string
			Expect(p.db.QueryRowContext(ctx, `SELECT "VM" FROM vinfo WHERE "VM ID" = 'vm1'`).Scan(&name)).To(Succeed())
			Expect(name).To(Equal("existing"))
			Expect(storedMetadata(GinkgoTB(), p.db, "vm1")).To(Equal(expected))
			Expect(storedMetadata(GinkgoTB(), p.db, "vm2")).To(Equal(map[string][]string{"Owner": {"Platform"}}))
		},
		Entry("merges metadata for the same vCenter",
			"false", "Finance", "uuid", map[string][]string{"Owner": {"Finance", "Payments"}},
		),

		Entry("ignores incoming template metadata",
			"true", "Finance", "uuid", map[string][]string{"Owner": {"Finance"}},
		),

		Entry("adds metadata to an existing VM without an owner",
			"false", "", "uuid", map[string][]string{"Owner": {"Payments"}},
		),

		Entry("keeps metadata from another vCenter separate",
			"false", "Finance", "other-uuid", map[string][]string{"Owner": {"Finance"}},
		),
	)

	It("stops later writes after malformed RVTools metadata", func() {
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

	Context("formatted blank cells", func() {
		var vms []models.VM

		BeforeEach(func() {
			rows := []map[string]string{
				{
					"VM":          "vm",
					"VM ID":       "vm1",
					"VI SDK UUID": "uuid",
					"Host":        "host",
					"Cluster":     "cluster",
					"Datacenter":  "dc",
					"Annotation":  "notes",
					"Blank":       "replace-with-formatted-blank",
					"Real":        "VM",
					"Text":        "  Finance's; [QA]  ",
				},
				{
					"VM":          "empty",
					"VM ID":       "vm2",
					"VI SDK UUID": "uuid",
					"Host":        "host",
					"Cluster":     "cluster",
					"Datacenter":  "dc",
				},
				{
					"VM":          "template",
					"VM ID":       "vm3",
					"VI SDK UUID": "uuid",
					"Host":        "host",
					"Cluster":     "cluster",
					"Datacenter":  "dc",
					"Template":    "true",
					"Real":        "excluded template value",
				},
			}
			sheets := defaultStandardSheets(rows, []map[string]string{{
				"Host":       "host",
				"Cluster":    "cluster",
				"Datacenter": "dc",
			}})
			sheets[0].Headers = []string{"VM", "VM ID", "VI SDK UUID", "Host", "Cluster", "CPUs", "Memory", "Powerstate", "Template",
				"Annotation", "Blank", "Real", "Text", "Datacenter"}
			path := createTestExcel(GinkgoTB(), sheets...)
			source, err := zip.OpenReader(path)
			Expect(err).To(Succeed())
			DeferCleanup(source.Close)

			var buffer bytes.Buffer
			writer := zip.NewWriter(&buffer)
			replaced := false
			for _, file := range source.File {
				reader, err := file.Open()
				Expect(err).To(Succeed())
				DeferCleanup(reader.Close)

				data, err := io.ReadAll(reader)
				Expect(err).To(Succeed())
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

			path = filepath.Join(GinkgoT().TempDir(), "formatted-blanks.xlsx")
			Expect(os.WriteFile(path, buffer.Bytes(), 0600)).To(Succeed())

			result, err := p.IngestRvTools(context.Background(), path)
			Expect(err).To(Succeed())
			Expect(result.HasErrors()).To(BeFalse())

			vms, err = p.VMs(context.Background(), Filters{}, Options{})
			Expect(err).To(Succeed())
		})

		It("excludes templates", func() {
			Expect(vms).To(HaveLen(2))
			Expect([]string{vms[0].ID, vms[1].ID}).To(ConsistOf("vm1", "vm2"))
		})

		It("preserves genuine metadata values", func() {
			Expect(vms).To(ContainElement(And(
				HaveField("ID", "vm1"),
				HaveField("Metadata", Equal(models.Metadata{
					"Real": {"VM"},
					"Text": {"  Finance's; [QA]  "},
				})),
			)))
		})

		It("omits blank metadata", func() {
			Expect(vms).To(ContainElement(And(
				HaveField("ID", "vm2"),
				HaveField("Metadata", BeEmpty()),
			)))
		})
	})

	It("stops later statements when metadata persistence fails", func() {
		err := p.executeStatements(context.Background(), `INSERT INTO vm_metadata (vm_id, key, value) VALUES ('missing', 'Owner', 'Finance');
			INSERT INTO vinfo ("VM ID", "VM") VALUES ('later', 'later');`)
		Expect(err).To(HaveOccurred())

		var count int
		Expect(db.QueryRow(`SELECT count(*) FROM vinfo`).Scan(&count)).To(Succeed())
		Expect(count).To(BeZero(), "failed metadata persistence must stop later statements")
	})

	It("clears metadata and storage details before parser reuse", func() {
		_, err := p.db.ExecContext(ctx, `INSERT INTO vinfo ("VM ID", "VM") VALUES ('vm1', 'vm');
			INSERT INTO vm_metadata (vm_id, key, value) VALUES ('vm1', 'Owner', 'Alice');
			INSERT INTO vhostscsi ("Host ID", "Canonical Name", "LUN Vendor", "LUN Model")
			VALUES ('host-1', 'naa.shared', 'OLD', 'old-model')`)
		Expect(err).To(Succeed())

		Expect(p.Cleanup()).To(Succeed())
		Expect(storedMetadata(GinkgoTB(), p.db, "vm1")).To(BeEmpty())

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

var _ = Describe("RVTools metadata inputs", func() {
	It("reports a missing workbook", func() {
		_, err := readRVToolsMetadata(context.Background(), filepath.Join(GinkgoT().TempDir(), "missing.xlsx"))
		Expect(err).To(MatchError(ContainSubstring("opening RVTools workbook")))
	})

	It("reports a missing vInfo sheet", func() {
		path := createTestExcel(GinkgoTB(), NewExcelSheet("vHost", []string{"Host"}, nil))

		_, err := readRVToolsMetadata(context.Background(), path)
		Expect(err).To(MatchError(ContainSubstring("reading vInfo headers")))
	})

	It("ignores an empty vInfo header", func() {
		path := createTestExcel(GinkgoTB(), NewExcelSheet("vInfo", nil, nil))

		metadata, err := readRVToolsMetadata(context.Background(), path)
		Expect(err).NotTo(HaveOccurred())
		Expect(metadata).To(BeEmpty())
	})

	It("omits metadata SQL when no metadata was read", func() {
		query, err := NewBuilder().IngestRvtoolsQuery("not-opened.xlsx", false)
		Expect(err).NotTo(HaveOccurred())
		Expect(query).NotTo(ContainSubstring("WITH metadata AS ("))
	})
})

var _ = Describe("VM metadata in older inventories", func() {
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
		Expect(vms).To(HaveLen(1))
		Expect(vms[0].Metadata).To(BeEmpty())

		var count int
		Expect(db.QueryRow(`SELECT count(*) FROM information_schema.tables WHERE table_name='vm_metadata'`).Scan(&count)).To(Succeed())
		Expect(count).To(BeZero())
	})
})

var _ = Describe("RVTools validation errors", func() {
	DescribeTable("accepts results without errors",
		func(result ValidationResult) {
			Expect(result.Error()).NotTo(HaveOccurred())
		},
		Entry("no issues", ValidationResult{}),
		Entry("warnings only", ValidationResult{
			Warnings: []ValidationIssue{{Message: "warning"}},
		}),
	)

	It("preserves technical details for the caller's fallback", func() {
		result := ValidationResult{Errors: []ValidationIssue{
			{Code: CodeMissingVMID, Message: "First issue: missing VM ID"},
			{Code: CodeColumnValidationFailed, Message: "SELECT private_column FROM /private/database"},
			{Code: CodeMissingVMName, Message: "Second issue: missing name"},
		}}

		err := fmt.Errorf("ingesting: %w", result.Error())
		var validation *SchemaValidationError
		Expect(errors.As(err, &validation)).To(BeFalse(), "technical issues must use the caller's fallback")
		Expect(err).To(MatchError(And(
			ContainSubstring("private_column"),
			ContainSubstring("First issue: missing VM ID"),
			ContainSubstring("Second issue: missing name"),
		)))
		Expect(result.Errors).To(HaveLen(3))
	})
})

var _ = Describe("RVTools workbook errors", func() {
	DescribeTable("preserves invalid ZIP errors",
		func(content string) {
			path := filepath.Join(GinkgoT().TempDir(), "private.xlsx")
			Expect(os.WriteFile(path, []byte(content), 0600)).To(Succeed())

			_, err := readRVToolsMetadata(context.Background(), path)
			Expect(err).To(MatchError(zip.ErrFormat))
		},
		Entry("JSON file", "{}"),
		Entry("empty file", ""),
	)

	It("preserves the missing sheet error", func() {
		path := createTestExcel(GinkgoTB(), NewExcelSheet("vHost", []string{"VM ID", "VM", "Cluster", "VI SDK UUID"}, nil))

		_, err := readRVToolsMetadata(context.Background(), path)
		Expect(err).To(HaveOccurred())

		var missing excelize.ErrSheetNotExist
		Expect(errors.As(err, &missing)).To(BeTrue())
		Expect(missing.SheetName).To(Equal("vInfo"))
	})

	It("rejects a truncated workbook", func() {
		path := createTestExcel(GinkgoTB(), NewExcelSheet("vInfo", []string{"VM ID", "VM", "Cluster", "VI SDK UUID"}, nil))

		data, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())

		Expect(os.Truncate(path, int64(len(data)/2))).To(Succeed())

		_, err = readRVToolsMetadata(context.Background(), path)
		Expect(err).To(MatchError(zip.ErrFormat))
	})

	It("preserves a missing file error", func() {
		path := filepath.Join(GinkgoT().TempDir(), "missing.xlsx")

		_, err := readRVToolsMetadata(context.Background(), path)
		Expect(err).To(MatchError(os.ErrNotExist))
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
