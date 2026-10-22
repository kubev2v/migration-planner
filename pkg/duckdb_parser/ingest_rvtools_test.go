package duckdb_parser

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValidationResultUserMessage(t *testing.T) {
	require.Empty(t, (ValidationResult{}).UserMessage())
	result := ValidationResult{Errors: []ValidationIssue{
		{Code: CodeColumnValidationFailed},
		{Code: CodeMissingVMID, Message: "first issue"},
		{Code: CodeColumnValidationFailed},
		{Code: CodeMissingCluster, Message: "second issue"},
		{Code: CodeColumnValidationFailed},
	}}
	require.Equal(t, "first issue\nsecond issue", result.UserMessage())
}

func TestValidationResultUnreadableTable(t *testing.T) {
	parser, _, cleanup := setupTestParser(t, nil)
	defer cleanup()
	var result ValidationResult
	parser.validateVinfoData(context.Background(), &result, "missing_private_table")
	require.Len(t, result.Errors, 1)
	require.Equal(t, CodeColumnValidationFailed, result.Errors[0].Code)
	require.Equal(t, "missing_private_table", result.Errors[0].Table)
	require.Contains(t, result.UserMessage(), "could not read vInfo table:")
	require.Contains(t, result.UserMessage(), "missing_private_table")
}

func TestTranslateXLSXError(t *testing.T) {
	for _, tc := range []struct {
		name, technical, want string
	}{
		{name: "missing workbook", technical: "query: No xl/workbook.xml found in /private/file.xlsx", want: "The file is corrupted or not a valid Excel file"},
		{name: "missing sheet", technical: `query: "vInfo" not found in xlsx file /private/file.xlsx`, want: "File is not a valid RVTools export (missing required 'vInfo' sheet)"},
		{name: "unknown", technical: "unexpected database failure at /private/file.xlsx", want: "unexpected database failure at /private/file.xlsx"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := errors.New(tc.technical)
			translated := translateXLSXError(original)
			require.EqualError(t, translated, tc.want)
			if tc.name == "unknown" {
				require.ErrorIs(t, translated, original)
			}
		})
	}
}

func TestIngestRvTools_ContextErrors(t *testing.T) {
	for _, tc := range []struct {
		name     string
		deadline bool
	}{{name: "canceled"}, {name: "timeout", deadline: true}} {
		t.Run(tc.name, func(t *testing.T) {
			parser, _, cleanup := setupTestParser(t, nil)
			defer cleanup()
			path := createTestExcelWithCustomHeaders(t, []string{"VM ID", "VM", "Cluster", "VI SDK UUID"}, nil)
			ctx, cancel := context.WithCancel(context.Background())
			if tc.deadline {
				cancel()
				ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			}
			cancel()
			_, err := parser.IngestRvTools(ctx, path)
			require.ErrorIs(t, err, ctx.Err())
		})
	}
}

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

func TestIngestRvTools_Failures(t *testing.T) {
	for _, tc := range []struct {
		name          string
		content       []byte
		sheet         string
		truncate      bool
		missingColumn bool
		want          string
	}{
		{name: "JSON", content: []byte(`{"VM":"vm-1"}`), want: "zip: not a valid zip file"},
		{name: "text", content: []byte("not an Excel workbook"), want: "zip: not a valid zip file"},
		{name: "empty", content: []byte{}, want: "zip: not a valid zip file"},
		{name: "truncated", truncate: true, want: "zip: not a valid zip file"},
		{name: "missing sheet", sheet: "vHost", want: "reading vInfo headers: sheet vInfo does not exist"},
		{name: "missing column", missingColumn: true, want: `could not validate column "VM ID":`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parser, db, cleanup := setupTestParser(t, nil)
			defer cleanup()
			sheet := tc.sheet
			if sheet == "" {
				sheet = "vInfo"
			}
			headers := []string{"VM ID", "VM", "Cluster", "VI SDK UUID"}
			if tc.missingColumn {
				headers = headers[1:]
			}
			path := createTestExcel(t, NewExcelSheet(sheet, headers, []map[string]string{{"VM ID": "vm-1", "VM": "vm-1", "Cluster": "cluster-1", "VI SDK UUID": "uuid"}}))
			if tc.content != nil {
				require.NoError(t, os.WriteFile(path, tc.content, 0600))
			}
			if tc.truncate {
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				require.NoError(t, os.Truncate(path, int64(len(data)/2)))
			}
			result, err := parser.IngestRvTools(context.Background(), path)
			require.ErrorContains(t, err, tc.want)
			if tc.missingColumn {
				require.True(t, result.HasErrors())
				require.Equal(t, CodeColumnValidationFailed, result.Errors[0].Code)
				require.Contains(t, result.Errors[0].Message, tc.want)
			}
			var count int
			require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'vinfo_raw'`).Scan(&count))
			require.Zero(t, count, "failed ingestion must not leave staging data")
		})
	}
}
