package duckdb_parser

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
)

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
