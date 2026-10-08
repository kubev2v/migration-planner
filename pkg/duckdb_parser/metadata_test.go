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

	duckdb "github.com/marcboeker/go-duckdb/v2"
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
	vms, err := p.VMs(ctx, Filters{}, Options{})
	require.NoError(t, err)
	require.Len(t, vms, 1)
	require.Equal(t, map[string][]string{
		"Owner's; \"name\"": {"HR", "Payments; QA", "Production"},
		"cat1":              {"  Finance & IT  "}, "cat2": {"QA"},
	}, vms[0].Metadata)
	var columnType string
	require.NoError(t, p.db.QueryRowContext(ctx, `SELECT typeof(metadata) FROM vinfo WHERE "VM ID" = 'vm1'`).Scan(&columnType))
	require.Equal(t, "MAP(VARCHAR, VARCHAR[])", columnType)
	require.Empty(t, vms[0].Labels)
	withMetadata, err := p.BuildInventory(ctx, nil)
	require.NoError(t, err)

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
	require.Zero(t, count, "VM fields and metadata must be inserted together")
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

type cancelMetadataWriteDB struct {
	*sql.DB
	cancel context.CancelFunc
}

func (db *cancelMetadataWriteDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if db.cancel != nil && (strings.HasPrefix(query, "UPDATE vinfo_raw SET metadata") || strings.HasPrefix(query, "WITH metadata AS (")) {
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
	p.db = &cancelMetadataWriteDB{DB: db, cancel: cancel}
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
	p.db = &cancelMetadataWriteDB{DB: db, cancel: cancel}
	_, err := p.IngestRvTools(ctx, path)
	require.ErrorIs(t, err, context.Canceled)
	var count int
	require.NoError(t, db.QueryRowContext(context.Background(), `SELECT count(*) FROM information_schema.tables WHERE table_name = 'vinfo_raw'`).Scan(&count))
	require.Zero(t, count, "cancelled import must drop its temporary table")
	_, err = p.IngestRvTools(context.Background(), path)
	require.NoError(t, err)
	var metadata duckdb.Composite[map[string][]string]
	require.NoError(t, db.QueryRowContext(context.Background(), `SELECT metadata FROM vinfo WHERE "VM ID" = 'vm1'`).Scan(&metadata))
	require.Equal(t, map[string][]string{"Owner": {"Finance"}}, metadata.Get())
}

func TestMetadataSQLiteExistingVM(t *testing.T) {
	for _, template := range []bool{false, true} {
		t.Run(fmt.Sprintf("incoming template=%t", template), func(t *testing.T) {
			ctx := context.Background()
			p, _, cleanup := setupTestParser(t, nil)
			defer cleanup()
			db, err := sql.Open("duckdb", "")
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			for _, incoming := range []bool{false, true} {
				vms := []sqliteVM{{id: "vm1", name: "existing", clusterName: "cluster"}}
				owner := "Finance"
				if incoming {
					vms[0].name, owner = "incoming", "Payments"
					vms = append(vms, sqliteVM{id: "vm2", name: "new", clusterName: "cluster"})
				}
				path := createTestSQLite(t, "uuid", []sqliteCluster{{id: "c1", name: "cluster", datacenter: "dc"}}, vms)
				_, err = db.Exec(`ATTACH '` + escapeSQLString(path) + `' AS src (TYPE sqlite);
					ALTER TABLE src.VM ADD COLUMN Tags VARCHAR`)
				require.NoError(t, err)
				tags, err := json.Marshal([]map[string]string{{"categoryID": "Owner", "name": owner}})
				require.NoError(t, err)
				_, err = db.Exec(`UPDATE src.VM SET Tags = ? WHERE ID = 'vm1'`, string(tags))
				require.NoError(t, err)
				if incoming {
					_, err = db.Exec(`UPDATE src.VM SET IsTemplate = ? WHERE ID = 'vm1'`, template)
					require.NoError(t, err)
					_, err = db.Exec(`UPDATE src.VM SET Tags = '[{"categoryID":"Owner","name":"Platform"}]' WHERE ID = 'vm2'`)
					require.NoError(t, err)
				}
				_, err = db.Exec(`DETACH src`)
				require.NoError(t, err)
				_, err = p.IngestSqlite(ctx, path)
				require.NoError(t, err)
			}
			var name string
			var metadata duckdb.Composite[map[string][]string]
			require.NoError(t, p.db.QueryRowContext(ctx, `SELECT "VM", metadata FROM vinfo WHERE "VM ID" = 'vm1'`).Scan(&name, &metadata))
			require.Equal(t, "existing", name)
			require.Equal(t, map[string][]string{"Owner": {"Finance"}}, metadata.Get())
			require.NoError(t, p.db.QueryRowContext(ctx, `SELECT metadata FROM vinfo WHERE "VM ID" = 'vm2'`).Scan(&metadata))
			require.Equal(t, map[string][]string{"Owner": {"Platform"}}, metadata.Get())
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
				if tc.name == "metadata column" {
					require.Equal(t, map[string][]string{"metadata": {"Original value"}}, got[0].Metadata)
				} else {
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
	}{
		{name: "two eligible rows"},
		{name: "first row excluded", firstTemplate: "true"},
		{name: "second row excluded", secondTemplate: "true"},
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
	for _, template := range []string{"false", "true"} {
		t.Run("incoming template="+template, func(t *testing.T) {
			p, _, cleanup := setupTestParser(t, nil)
			defer cleanup()
			ctx := context.Background()
			vms := []map[string]string{{"VM": "existing", "VM ID": "vm1", "VI SDK UUID": "uuid", "Host": "host",
				"Cluster": "cluster", "Datacenter": "dc", "Owner": "Finance"}}
			sheets := defaultStandardSheets(vms, []map[string]string{{"Host": "host", "Cluster": "cluster", "Datacenter": "dc"}})
			sheets[0].Headers = []string{"VM", "VM ID", "VI SDK UUID", "Host", "Cluster", "CPUs", "Memory", "Powerstate",
				"Template", "Annotation", "Owner", "Datacenter"}
			_, err := p.IngestRvTools(ctx, createTestExcel(t, sheets...))
			require.NoError(t, err)
			vms[0]["VM"], vms[0]["Owner"], vms[0]["Template"] = "incoming", "Payments", template
			sheets[0].Rows = append(vms, map[string]string{"VM": "new", "VM ID": "vm2", "VI SDK UUID": "uuid", "Host": "host",
				"Cluster": "cluster", "Datacenter": "dc", "Owner": "Platform"})
			_, err = p.IngestRvTools(ctx, createTestExcel(t, sheets...))
			require.NoError(t, err)
			var name string
			var metadata duckdb.Composite[map[string][]string]
			require.NoError(t, p.db.QueryRowContext(ctx, `SELECT "VM", metadata FROM vinfo WHERE "VM ID" = 'vm1'`).Scan(&name, &metadata))
			require.Equal(t, "existing", name)
			require.Equal(t, map[string][]string{"Owner": {"Finance"}}, metadata.Get())
			require.NoError(t, p.db.QueryRowContext(ctx, `SELECT metadata FROM vinfo WHERE "VM ID" = 'vm2'`).Scan(&metadata))
			require.Equal(t, map[string][]string{"Owner": {"Platform"}}, metadata.Get())
		})
	}
}

func TestRVToolsMetadataWriteFailure(t *testing.T) {
	p, db, cleanup := setupTestParser(t, nil)
	defer cleanup()
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `CREATE TABLE vinfo_raw (metadata MAP(VARCHAR, VARCHAR[]));
        INSERT INTO vinfo_raw VALUES (map())`)
	require.NoError(t, err)
	err = p.executeStatements(ctx, `UPDATE vinfo_raw SET metadata = (
		SELECT from_json(?, '"MAP(VARCHAR, VARCHAR[])"')); DELETE FROM vinfo_raw;`, "invalid JSON")
	require.ErrorContains(t, err, "Malformed JSON")
	var metadata duckdb.Composite[map[string][]string]
	require.NoError(t, db.QueryRowContext(ctx, `SELECT metadata FROM vinfo_raw`).Scan(&metadata))
	require.Empty(t, metadata.Get(), "statements after a failed metadata write must not run")
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
	require.NotContains(t, query, "UPDATE vinfo_raw SET metadata")
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
	schema = strings.Replace(schema, ",\n    \"metadata\" MAP(VARCHAR, VARCHAR[]) DEFAULT map()", "", 1)
	_, err = db.Exec(schema)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO vinfo ("VM ID", "VM") VALUES ('vm1', 'vm')`)
	require.NoError(t, err)
	vms, err := p.VMs(context.Background(), Filters{}, Options{})
	require.NoError(t, err)
	require.Empty(t, vms[0].Metadata)
	var count int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM information_schema.columns WHERE table_name='vinfo' AND column_name='metadata'`).Scan(&count))
	require.Zero(t, count)
}
