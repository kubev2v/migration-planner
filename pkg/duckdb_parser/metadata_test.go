package duckdb_parser

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kubev2v/migration-planner/pkg/store"
	"github.com/stretchr/testify/require"
)

func TestMetadataSQLite(t *testing.T) {
	ctx := context.Background()
	path := createTestSQLite(t, "uuid", []sqliteCluster{{id: "c1", name: "cluster", datacenter: "dc"}},
		[]sqliteVM{{id: "vm1", name: "vm", clusterName: "cluster"}})
	db, err := sql.Open("duckdb", "")
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	_, err = db.Exec(`ATTACH '` + escapeSQLString(path) + `' AS src (TYPE sqlite);
		ALTER TABLE src.VM ADD COLUMN Tags VARCHAR;
		ALTER TABLE src.VM ADD COLUMN CustomDef VARCHAR;
		ALTER TABLE src.VM ADD COLUMN CustomValues VARCHAR`)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE src.VM SET Tags = ?, CustomDef = ?, CustomValues = ?`,
		`[{"categoryID":"cat1","name":"Payments; QA"},{"categoryID":"cat1","name":"Production"},{"categoryID":"cat1","name":"Payments; QA"},{"categoryID":"cat2","name":"QA"}]`,
		`[{"key":1,"name":"cat1"},{"key":2,"name":"Owner's; \"name\""},{"key":3,"name":"Empty"}]`,
		`[{"key":1,"value":"  Finance & IT  "},{"key":2,"value":"HR"},{"key":3,"value":" "},{"key":99,"value":"unmatched"}]`)
	require.NoError(t, err)
	_, err = db.Exec(`DETACH src`)
	require.NoError(t, err)
	p, _, cleanup := setupTestParser(t, nil)
	defer cleanup()
	_, err = p.db.ExecContext(ctx, `CREATE TABLE tag_categories (id VARCHAR); INSERT INTO tag_categories VALUES ('permanent')`)
	require.NoError(t, err)
	_, err = p.db.ExecContext(ctx, `CREATE TEMP TABLE tag_categories (id VARCHAR, name VARCHAR)`)
	require.NoError(t, err)
	_, err = p.db.ExecContext(ctx, `INSERT INTO temp.main.tag_categories VALUES (?, ?)`, "cat1", "Owner's; \"name\"")
	require.NoError(t, err)
	result, err := p.IngestSqlite(ctx, path)
	require.NoError(t, err)
	require.False(t, result.HasErrors())
	var temporaryTables int
	require.NoError(t, p.db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_catalog = 'temp' AND table_name = 'tag_categories'`).Scan(&temporaryTables))
	require.Zero(t, temporaryTables, "category staging must be removed after import")
	var categoryID string
	require.NoError(t, p.db.QueryRowContext(ctx, `SELECT id FROM tag_categories`).Scan(&categoryID))
	require.Equal(t, "permanent", categoryID, "cleanup must preserve a permanent table with the same name")
	vms, err := p.VMs(ctx, Filters{}, Options{})
	require.NoError(t, err)
	require.Len(t, vms, 1)
	require.Equal(t, map[string][]string{
		"Owner's; \"name\"": {"HR", "Payments; QA", "Production"},
		"cat1":              {"  Finance & IT  "},
	}, vms[0].Metadata)
	var count int
	require.NoError(t, p.db.QueryRowContext(ctx, `SELECT count(*) FROM vm_metadata WHERE vm_id = 'vm1'`).Scan(&count))
	require.Equal(t, 4, count)
	require.NoError(t, p.db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_name='vinfo' AND column_name='metadata'`).Scan(&count))
	require.Zero(t, count)
	require.Empty(t, vms[0].Labels)
	withMetadata, err := p.BuildInventory(ctx, nil)
	require.NoError(t, err)
	for _, categoryName := range []string{"", " ", "unavailable"} {
		t.Run("category name="+categoryName, func(t *testing.T) {
			unresolved, _, closeUnresolved := setupTestParser(t, nil)
			defer closeUnresolved()
			if categoryName != "unavailable" {
				_, err := unresolved.db.ExecContext(ctx, `CREATE TEMP TABLE tag_categories (id VARCHAR, name VARCHAR);
					INSERT INTO tag_categories VALUES ('cat1', ?)`, categoryName)
				require.NoError(t, err)
			}
			result, err := unresolved.IngestSqlite(ctx, path)
			require.NoError(t, err)
			require.False(t, result.HasErrors())
			vms, err := unresolved.VMs(ctx, Filters{}, Options{})
			require.NoError(t, err)
			require.Len(t, vms, 1)
			require.Equal(t, map[string][]string{
				"cat1": {"  Finance & IT  "}, "Owner's; \"name\"": {"HR"},
			}, vms[0].Metadata, "unresolved tags must be omitted without losing custom attributes")
		})
	}

	_, err = db.Exec(`ATTACH '` + escapeSQLString(path) + `' AS src (TYPE sqlite);
		UPDATE src.VM SET Tags = '[]', CustomValues = '[]'; DETACH src`)
	require.NoError(t, err)
	fresh, _, closeFresh := setupTestParser(t, nil)
	defer closeFresh()
	_, err = fresh.IngestSqlite(ctx, path)
	require.NoError(t, err)
	vms, err = fresh.VMs(ctx, Filters{}, Options{})
	require.NoError(t, err)
	require.Empty(t, vms[0].Metadata)
	withoutMetadata, err := fresh.BuildInventory(ctx, nil)
	require.NoError(t, err)
	withMetadata.CreatedAt, withoutMetadata.CreatedAt = nil, nil
	require.Equal(t, withMetadata, withoutMetadata, "metadata must not change aggregate inventory")
}

func TestSQLiteMetadataWriteFailure(t *testing.T) {
	path := createTestSQLite(t, "uuid", []sqliteCluster{{id: "c1", name: "cluster", datacenter: "dc"}},
		[]sqliteVM{{id: "vm1", name: "vm", clusterName: "cluster"}})
	db, err := sql.Open("duckdb", "")
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	_, err = db.Exec(`ATTACH '` + escapeSQLString(path) + `' AS src (TYPE sqlite);
		ALTER TABLE src.VM ADD COLUMN Tags VARCHAR;
		UPDATE src.VM SET Tags = 'invalid JSON'; DETACH src`)
	require.NoError(t, err)
	p, _, cleanup := setupTestParser(t, nil)
	defer cleanup()
	_, err = p.db.ExecContext(context.Background(), `CREATE TEMP TABLE tag_categories AS SELECT 'cat1' AS id, 'Owner' AS name`)
	require.NoError(t, err)
	_, err = p.IngestSqlite(context.Background(), path)
	require.ErrorContains(t, err, "Malformed JSON")
	var count int
	require.NoError(t, p.db.QueryRowContext(context.Background(), `SELECT count(*) FROM vinfo`).Scan(&count))
	require.Equal(t, 1, count, "VM insert precedes metadata, but failed ingestion must not be published")
	require.NoError(t, p.db.QueryRowContext(context.Background(), `SELECT count(*) FROM vcpu`).Scan(&count))
	require.Zero(t, count, "metadata failure must stop later inventory writes")
	require.NoError(t, p.db.QueryRowContext(context.Background(), `SELECT count(*) FROM duckdb_databases() WHERE database_name = 'src'`).Scan(&count))
	require.Zero(t, count, "failed import must detach its SQLite source")
	require.NoError(t, p.db.QueryRowContext(context.Background(), `SELECT count(*) FROM information_schema.tables WHERE table_catalog = 'temp' AND table_name = 'tag_categories'`).Scan(&count))
	require.Zero(t, count, "failed import must drop category staging")
	validPath := createTestSQLite(t, "uuid", []sqliteCluster{{id: "c1", name: "cluster", datacenter: "dc"}},
		[]sqliteVM{{id: "vm2", name: "replacement", clusterName: "cluster"}})
	_, err = p.IngestSqlite(context.Background(), validPath)
	require.NoError(t, err)
	var name string
	require.NoError(t, p.db.QueryRowContext(context.Background(), `SELECT "VM" FROM vinfo WHERE "VM ID" = 'vm2'`).Scan(&name))
	require.Equal(t, "replacement", name)
}

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

func TestSQLiteCancelledImportRetry(t *testing.T) {
	p, db, cleanup := setupTestParser(t, nil)
	defer cleanup()
	path := createTestSQLite(t, "uuid", []sqliteCluster{{id: "c1", name: "cluster", datacenter: "dc"}},
		[]sqliteVM{{id: "vm1", name: "vm", clusterName: "cluster"}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := db.ExecContext(ctx, `CREATE TEMP TABLE tag_categories AS SELECT 'cat1' AS id, 'Owner' AS name`)
	require.NoError(t, err)
	p.db = &cancelVMWriteDB{DB: db, cancel: cancel}
	_, err = p.IngestSqlite(ctx, path)
	require.ErrorIs(t, err, context.Canceled)
	var count int
	require.NoError(t, db.QueryRowContext(context.Background(), `SELECT count(*) FROM information_schema.tables WHERE table_catalog = 'temp' AND table_name = 'tag_categories'`).Scan(&count))
	require.Zero(t, count)
	require.NoError(t, db.QueryRowContext(context.Background(), `SELECT count(*) FROM duckdb_databases() WHERE database_name = 'src'`).Scan(&count))
	require.Zero(t, count)
	_, err = p.IngestSqlite(context.Background(), path)
	require.NoError(t, err)
}

func TestRVToolsCancelledImportRetry(t *testing.T) {
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
	require.ErrorIs(t, err, context.Canceled)
	var count int
	require.NoError(t, db.QueryRowContext(context.Background(), `SELECT count(*) FROM information_schema.tables WHERE table_name = 'vinfo_raw'`).Scan(&count))
	require.Zero(t, count, "cancelled import must drop its temporary table")
	_, err = p.IngestRvTools(context.Background(), path)
	require.NoError(t, err)
	require.Equal(t, map[string][]string{"Owner": {"Finance"}}, storedMetadata(t, db, "vm1"))
}

func TestMetadataSQLiteExistingVM(t *testing.T) {
	for _, tc := range []struct {
		template        bool
		owner           string
		differentCenter bool
	}{{false, "Finance", false}, {true, "Finance", false}, {false, "", false}, {false, "Finance", true}} {
		t.Run(fmt.Sprintf("incoming template=%t, existing owner=%q, different vCenter=%t", tc.template, tc.owner, tc.differentCenter), func(t *testing.T) {
			ctx := context.Background()
			p, _, cleanup := setupTestParser(t, nil)
			defer cleanup()
			db, err := sql.Open("duckdb", "")
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
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
				require.NoError(t, err)
				tags, err := json.Marshal([]map[string]string{{"categoryID": "Owner", "name": owner}})
				require.NoError(t, err)
				_, err = db.Exec(`UPDATE src.VM SET Tags = ? WHERE ID = 'vm1'`, string(tags))
				require.NoError(t, err)
				if incoming {
					_, err = db.Exec(`UPDATE src.VM SET IsTemplate = ? WHERE ID = 'vm1'`, tc.template)
					require.NoError(t, err)
					_, err = db.Exec(`UPDATE src.VM SET Tags = '[{"categoryID":"Owner","name":"Platform"}]' WHERE ID = 'vm2'`)
					require.NoError(t, err)
				}
				_, err = db.Exec(`DETACH src`)
				require.NoError(t, err)
				_, err = p.db.ExecContext(ctx, `CREATE TEMP TABLE tag_categories AS SELECT 'Owner' AS id, 'Owner' AS name`)
				require.NoError(t, err)
				_, err = p.IngestSqlite(ctx, path)
				require.NoError(t, err)
			}
			var name string
			require.NoError(t, p.db.QueryRowContext(ctx, `SELECT "VM" FROM vinfo WHERE "VM ID" = 'vm1'`).Scan(&name))
			require.Equal(t, "existing", name)
			expected := map[string][]string{}
			if tc.owner != "" {
				expected["Owner"] = []string{tc.owner}
			}
			if !tc.template && !tc.differentCenter {
				expected["Owner"] = append(expected["Owner"], "Payments")
			}
			require.Equal(t, expected, storedMetadata(t, p.db, "vm1"))
			require.Equal(t, map[string][]string{"Owner": {"Platform"}}, storedMetadata(t, p.db, "vm2"))
		})
	}
}

func TestMetadataRVTools(t *testing.T) {
	for _, tc := range []struct {
		name  string
		block []string
		want  bool
	}{
		{"supported", []string{"Annotation", "Team's; \"name\" ", "Blank", "Datacenter"}, true},
		{"missing annotation", []string{"Team's; \"name\" ", "Datacenter"}, false},
		{"missing datacenter", []string{"Annotation", "Team's; \"name\" "}, false},
		{"reordered", []string{"Datacenter", "Team's; \"name\" ", "Annotation"}, false},
		{"duplicate boundary", []string{"Annotation", "Team's; \"name\" ", "Annotation", "Datacenter"}, false},
		{"duplicate datacenter", []string{"Annotation", "Team's; \"name\" ", "Datacenter", "Datacenter"}, false},
		{"duplicate metadata header", []string{"Annotation", "Team's; \"name\" ", "Team's; \"name\" ", "Datacenter"}, true},
		{"metadata column", []string{"Annotation", "metadata", "Datacenter"}, true},
		{"no metadata", []string{"Annotation", "Datacenter"}, false},
		{"older tags", []string{"Annotation", "Datacenter", "VI SDK API Version", "Team's; \"name\" ", "Blank", "VI SDK Server"}, true},
		{"older attributes and tags", []string{"Annotation", "metadata", "Datacenter", "VI SDK API Version", "Team's; \"name\" ", "VI SDK Server"}, true},
		{"older missing server", []string{"Annotation", "Datacenter", "VI SDK API Version", "Team's; \"name\" "}, false},
		{"older reordered", []string{"Annotation", "Datacenter", "VI SDK Server", "Team's; \"name\" ", "VI SDK API Version"}, false},
		{"older duplicate boundary", []string{"Annotation", "Datacenter", "VI SDK API Version", "Team's; \"name\" ", "VI SDK API Version", "VI SDK Server"}, false},
		{"older duplicate server", []string{"Annotation", "Datacenter", "VI SDK API Version", "Team's; \"name\" ", "VI SDK Server", "VI SDK Server"}, false},
		{"modern sdk columns", []string{"Annotation", "Team's; \"name\" ", "Datacenter", "VI SDK API Version", "VI SDK Server"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, _, cleanup := setupTestParser(t, nil)
			defer cleanup()
			headers := []string{"VM", "VM ID", "VI SDK UUID", "Host", "Cluster", "CPUs", "Memory", "Powerstate"}
			headers = append(headers, tc.block...)
			vms := []map[string]string{{"VM": "vm", "VM ID": "vm1", "VI SDK UUID": "uuid", "Host": "host", "Cluster": "cluster",
				"Datacenter": "dc", "Annotation": "free text", "Team's; \"name\" ": "  Payments; QA, [x]  ", "Blank": "  ", "metadata": "Original value"}}
			sheets := defaultStandardSheets(vms, []map[string]string{{"Host": "host", "Cluster": "cluster", "Datacenter": "dc"}})
			sheets[0].Headers = headers
			result, err := p.IngestRvTools(context.Background(), createTestExcel(t, sheets...))
			require.NoError(t, err)
			require.False(t, result.HasErrors())
			got, err := p.VMs(context.Background(), Filters{}, Options{})
			require.NoError(t, err)
			require.Len(t, got, 1)
			if tc.want {
				switch tc.name {
				case "metadata column":
					require.Equal(t, map[string][]string{"metadata": {"Original value"}}, got[0].Metadata)
				case "older attributes and tags":
					require.Equal(t, map[string][]string{"metadata": {"Original value"}, "Team's; \"name\" ": {"  Payments; QA, [x]  "}}, got[0].Metadata)
				default:
					require.Equal(t, map[string][]string{"Team's; \"name\" ": {"  Payments; QA, [x]  "}}, got[0].Metadata)
				}
			} else {
				require.Empty(t, got[0].Metadata)
			}
			require.Empty(t, got[0].Labels)
		})
	}
}

func TestMetadataRVToolsDuplicateVMIDs(t *testing.T) {
	for _, tc := range []struct {
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
	} {
		t.Run(tc.name, func(t *testing.T) {
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
			require.NoError(t, err)
			require.False(t, result.HasErrors())
			got, err := p.VMs(ctx, Filters{}, Options{})
			require.NoError(t, err)
			require.Len(t, got, 2)
			for _, vm := range got {
				if vm.ID == "vm1" {
					require.Empty(t, vm.Metadata, "duplicate source IDs must not receive ambiguous metadata")
					if tc.firstTemplate == "true" {
						require.Equal(t, "second", vm.Name)
					} else if tc.secondTemplate == "true" {
						require.Equal(t, "first", vm.Name)
					}
				} else {
					require.Equal(t, "vm2", vm.ID)
					require.Equal(t, map[string][]string{"Owner": {"Platform"}}, vm.Metadata)
				}
			}
		})
	}
}

func TestMetadataRVToolsExistingVM(t *testing.T) {
	for _, tc := range []struct {
		template, owner string
		differentCenter bool
	}{{"false", "Finance", false}, {"true", "Finance", false}, {"false", "", false}, {"false", "Finance", true}} {
		t.Run(fmt.Sprintf("incoming template=%s, existing owner=%s, different vCenter=%t", tc.template, tc.owner, tc.differentCenter), func(t *testing.T) {
			p, _, cleanup := setupTestParser(t, nil)
			defer cleanup()
			ctx := context.Background()
			vms := []map[string]string{{"VM": "existing", "VM ID": "vm1", "VI SDK UUID": "uuid", "Host": "host",
				"Cluster": "cluster", "Datacenter": "dc", "Owner": tc.owner}}
			sheets := defaultStandardSheets(vms, []map[string]string{{"Host": "host", "Cluster": "cluster", "Datacenter": "dc"}})
			sheets[0].Headers = []string{"VM", "VM ID", "VI SDK UUID", "Host", "Cluster", "CPUs", "Memory", "Powerstate",
				"Template", "Annotation", "Owner", "Datacenter"}
			_, err := p.IngestRvTools(ctx, createTestExcel(t, sheets...))
			require.NoError(t, err)
			vms[0]["VM"], vms[0]["Owner"], vms[0]["Template"] = "incoming", "Payments", tc.template
			center := "uuid"
			if tc.differentCenter {
				center = "other-uuid"
			}
			vms[0]["VI SDK UUID"] = center
			sheets[0].Rows = append(vms, map[string]string{"VM": "new", "VM ID": "vm2", "VI SDK UUID": center, "Host": "host",
				"Cluster": "cluster", "Datacenter": "dc", "Owner": "Platform"})
			_, err = p.IngestRvTools(ctx, createTestExcel(t, sheets...))
			require.NoError(t, err)
			var name string
			require.NoError(t, p.db.QueryRowContext(ctx, `SELECT "VM" FROM vinfo WHERE "VM ID" = 'vm1'`).Scan(&name))
			require.Equal(t, "existing", name)
			expected := map[string][]string{}
			if tc.owner != "" {
				expected["Owner"] = []string{tc.owner}
			}
			if tc.template == "false" && !tc.differentCenter {
				expected["Owner"] = append(expected["Owner"], "Payments")
			}
			require.Equal(t, expected, storedMetadata(t, p.db, "vm1"))
			require.Equal(t, map[string][]string{"Owner": {"Platform"}}, storedMetadata(t, p.db, "vm2"))
		})
	}
}

func TestRVToolsMetadataWriteFailure(t *testing.T) {
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
	require.ErrorContains(t, err, "Malformed JSON")
	var count int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM vinfo`).Scan(&count))
	require.Equal(t, 1, count, "metadata failure must stop later statements")
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM vm_metadata`).Scan(&count))
	require.Zero(t, count)
}

func TestRVToolsMetadataReadErrors(t *testing.T) {
	ctx := context.Background()
	_, err := readRVToolsMetadata(ctx, t.TempDir()+"/missing.xlsx")
	require.ErrorContains(t, err, "opening RVTools workbook")

	missingSheet := createTestExcel(t, NewExcelSheet("vHost", []string{"Host"}, nil))
	_, err = readRVToolsMetadata(ctx, missingSheet)
	require.ErrorContains(t, err, "reading vInfo headers")

	emptySheet := createTestExcel(t, NewExcelSheet("vInfo", nil, nil))
	metadata, err := readRVToolsMetadata(ctx, emptySheet)
	require.NoError(t, err)
	require.Empty(t, metadata)

	query, err := NewBuilder().IngestRvtoolsQuery("not-opened.xlsx", false)
	require.NoError(t, err)
	require.NotContains(t, query, "WITH metadata AS (")
}

func TestMetadataRVToolsFormattedBlanks(t *testing.T) {
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
	require.NoError(t, err)
	defer func() { _ = source.Close() }()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	replaced := false
	for _, file := range source.File {
		reader, err := file.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.NoError(t, reader.Close())
		if file.Name == "xl/worksheets/sheet2.xml" {
			// RVTools emits shared-string cells without a value for formatted blanks.
			pattern := regexp.MustCompile(`<c\b[^>]*\br="K2"[^>]*>.*?</c>`)
			require.True(t, pattern.Match(data))
			data = pattern.ReplaceAll(data, []byte(`<c r="K2" s="0" t="s"/>`))
			replaced = true
		}
		entry, err := writer.CreateHeader(&file.FileHeader)
		require.NoError(t, err)
		_, err = entry.Write(data)
		require.NoError(t, err)
	}
	require.True(t, replaced)
	require.NoError(t, writer.Close())
	path = filepath.Join(t.TempDir(), "formatted-blanks.xlsx")
	require.NoError(t, os.WriteFile(path, buffer.Bytes(), 0600))

	p, _, cleanup := setupTestParser(t, nil)
	defer cleanup()
	result, err := p.IngestRvTools(context.Background(), path)
	require.NoError(t, err)
	require.False(t, result.HasErrors())
	got, err := p.VMs(context.Background(), Filters{}, Options{})
	require.NoError(t, err)
	require.Len(t, got, 2, "templates must remain excluded")
	for _, vm := range got {
		if vm.ID == "vm1" {
			require.Equal(t, map[string][]string{"Real": {"VM"}, "Text": {"  Finance's; [QA]  "}}, vm.Metadata)
		} else {
			require.Empty(t, vm.Metadata)
		}
	}
}

func TestMetadataHistorical(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	p := New(db, nil)
	schema, err := p.builder.CreateSchemaQuery()
	require.NoError(t, err)
	schema = regexp.MustCompile(`(?s)CREATE TABLE IF NOT EXISTS vm_metadata \(.*?\);`).ReplaceAllString(schema, "")
	_, err = db.Exec(schema)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO vinfo ("VM ID", "VM") VALUES ('vm1', 'vm')`)
	require.NoError(t, err)
	vms, err := p.VMs(context.Background(), Filters{}, Options{})
	require.NoError(t, err)
	require.Empty(t, vms[0].Metadata)
	var count int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM information_schema.tables WHERE table_name='vm_metadata'`).Scan(&count))
	require.Zero(t, count)
}

func storedMetadata(t *testing.T, db store.QueryInterceptor, id string) map[string][]string {
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

func TestMetadataPersistenceFailure(t *testing.T) {
	p, db, cleanup := setupTestParser(t, nil)
	defer cleanup()
	err := p.executeStatements(context.Background(), `INSERT INTO vm_metadata (vm_id, key, value) VALUES ('missing', 'Owner', 'Finance');
        INSERT INTO vinfo ("VM ID", "VM") VALUES ('later', 'later');`)
	require.Error(t, err)
	var count int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM vinfo`).Scan(&count))
	require.Zero(t, count, "failed metadata persistence must stop later statements")
}
