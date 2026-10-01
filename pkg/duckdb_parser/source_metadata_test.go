package duckdb_parser

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kubev2v/migration-planner/pkg/duckdb_parser/models"
)

func TestSourceMetadataSQLite(t *testing.T) {
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
		`[{"id":"tag1","categoryID":"cat1","name":"Payments; QA","description":"desc","usedBy":["owner"]}]`,
		`[{"key":1,"name":"cat1"},{"key":2,"name":"Empty"}]`,
		`[{"key":1,"value":"  Finance & IT  "},{"key":2,"value":" "},{"key":99,"value":"unmatched"}]`)
	require.NoError(t, err)
	_, err = db.Exec(`DETACH src`)
	require.NoError(t, err)
	p, _, cleanup := setupTestParser(t, nil)
	defer cleanup()
	result, err := p.IngestSqlite(ctx, path)
	require.NoError(t, err)
	require.False(t, result.HasErrors())
	vms, err := p.VMs(ctx, Filters{}, Options{})
	require.NoError(t, err)
	require.Len(t, vms, 1)
	require.ElementsMatch(t, models.SourceMetadata{
		{Key: "cat1", Value: "Payments; QA", Kind: "tag"},
		{Key: "cat1", Value: "  Finance & IT  ", Kind: "customAttribute"},
	}, vms[0].SourceMetadata)
	var raw string
	require.NoError(t, p.db.QueryRowContext(ctx, `SELECT source_metadata FROM vinfo WHERE "VM ID" = 'vm1'`).Scan(&raw))
	var entries []map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &entries))
	for _, entry := range entries {
		require.Len(t, entry, 3, "stored metadata must contain only key, value, and kind")
	}
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
	require.Empty(t, vms[0].SourceMetadata)
	withoutMetadata, err := fresh.BuildInventory(ctx, nil)
	require.NoError(t, err)
	withMetadata.CreatedAt, withoutMetadata.CreatedAt = nil, nil
	require.Equal(t, withMetadata, withoutMetadata, "source metadata must not change aggregate inventory")
}

func TestSourceMetadataRVTools(t *testing.T) {
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
		{"no metadata", []string{"Annotation", "Datacenter"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, _, cleanup := setupTestParser(t, nil)
			defer cleanup()
			headers := []string{"VM", "VM ID", "VI SDK UUID", "Host", "Cluster", "CPUs", "Memory", "Powerstate"}
			headers = append(headers, tc.block...)
			vms := []map[string]string{{"VM": "vm", "VM ID": "vm1", "VI SDK UUID": "uuid", "Host": "host", "Cluster": "cluster",
				"Datacenter": "dc", "Annotation": "free text", "Team's; \"name\" ": "  Payments; QA, [x]  ", "Blank": "  "}}
			sheets := defaultStandardSheets(vms, []map[string]string{{"Host": "host", "Cluster": "cluster", "Datacenter": "dc"}})
			sheets[0].Headers = headers
			result, err := p.IngestRvTools(context.Background(), createTestExcel(t, sheets...))
			require.NoError(t, err)
			require.False(t, result.HasErrors())
			got, err := p.VMs(context.Background(), Filters{}, Options{})
			require.NoError(t, err)
			require.Len(t, got, 1)
			if tc.want {
				var expected models.SourceMetadata
				for _, header := range tc.block {
					if header == "Team's; \"name\" " {
						expected = append(expected, models.SourceMetadataEntry{Key: header, Value: "  Payments; QA, [x]  ", Kind: "unknown"})
					}
				}
				require.ElementsMatch(t, expected, got[0].SourceMetadata)
			} else {
				require.Empty(t, got[0].SourceMetadata)
			}
			require.Empty(t, got[0].Labels)
		})
	}
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
	require.NotContains(t, query, "UPDATE vinfo SET source_metadata")
}

func TestSourceMetadataRVToolsFormattedBlanks(t *testing.T) {
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
			require.ElementsMatch(t, models.SourceMetadata{
				{Key: "Real", Value: "VM", Kind: "unknown"},
				{Key: "Text", Value: "  Finance's; [QA]  ", Kind: "unknown"},
			}, vm.SourceMetadata)
		} else {
			require.Empty(t, vm.SourceMetadata)
		}
	}
}

func TestSourceMetadataHistorical(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	p := New(db, nil)
	schema, err := p.builder.CreateSchemaQuery()
	require.NoError(t, err)
	schema = strings.Replace(schema, ",\n    \"source_metadata\" VARCHAR DEFAULT '[]'", "", 1)
	_, err = db.Exec(schema)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO vinfo ("VM ID", "VM") VALUES ('vm1', 'vm')`)
	require.NoError(t, err)
	vms, err := p.VMs(context.Background(), Filters{}, Options{})
	require.NoError(t, err)
	require.Empty(t, vms[0].SourceMetadata)
	var count int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM information_schema.columns WHERE table_name='vinfo' AND column_name='source_metadata'`).Scan(&count))
	require.Zero(t, count)
	var scanned models.SourceMetadata
	require.NoError(t, scanned.Scan(nil))
	require.Error(t, scanned.Scan(`broken`))
	require.Error(t, scanned.Scan(123))
	require.NoError(t, scanned.Scan(`[]`))
}
