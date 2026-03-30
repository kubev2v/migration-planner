package duckdb_parser

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"
)

// ValidationResult contains errors and warnings from schema validation.
// Errors indicate the inventory cannot be built.
// Warnings indicate the inventory can be built but with missing data.
type ValidationResult struct {
	Errors   []ValidationIssue
	Warnings []ValidationIssue
}

// ValidationIssue represents a single validation problem.
type ValidationIssue struct {
	Code    string // Machine-readable code (e.g., "NO_VMS", "EMPTY_HOSTS")
	Table   string // Affected table name
	Column  string // Affected column name (if applicable)
	Message string // Human-readable description
}

// HasErrors returns true if there are any validation errors.
func (v ValidationResult) HasErrors() bool {
	return len(v.Errors) > 0
}

// HasWarnings returns true if there are any validation warnings.
func (v ValidationResult) HasWarnings() bool {
	return len(v.Warnings) > 0
}

// IsValid returns true if there are no errors (warnings are acceptable).
func (v ValidationResult) IsValid() bool {
	return !v.HasErrors()
}

// Error returns a combined error message if there are errors, nil otherwise.
func (v ValidationResult) Error() error {
	if !v.HasErrors() {
		return nil
	}
	msg := "schema validation failed:"
	for _, e := range v.Errors {
		msg += fmt.Sprintf("\n  - [%s] %s", e.Code, e.Message)
	}
	return fmt.Errorf("%s", msg)
}

// Validation codes for errors
const (
	CodeNoVMs                  = "NO_VMS"
	CodeMissingVMID            = "MISSING_VM_ID"
	CodeMissingVMName          = "MISSING_VM_NAME"
	CodeMissingCluster         = "MISSING_CLUSTER"
	CodeMissingVISDKUUID       = "MISSING_VI_SDK_UUID"
	CodeColumnValidationFailed = "COLUMN_VALIDATION_FAILED"
)

// Validation codes for warnings
const (
	CodeEmptyHosts      = "EMPTY_HOSTS"
	CodeEmptyDatastores = "EMPTY_DATASTORES"
	CodeEmptyNetworks   = "EMPTY_NETWORKS"
	CodeEmptyCPU        = "EMPTY_CPU"
	CodeEmptyMemory     = "EMPTY_MEMORY"
	CodeEmptyDisks      = "EMPTY_DISKS"
	CodeEmptyNICs       = "EMPTY_NICS"
)

// warningChecks defines optional table checks that produce warnings (non-fatal).
// Each entry verifies that a table has at least one row.
var warningChecks = []struct {
	table   string
	code    string
	message string
}{
	{"vhost", CodeEmptyHosts, "No host data found - host information will be unavailable in inventory"},
	{"vdatastore", CodeEmptyDatastores, "No datastore data found - storage information will be unavailable in inventory"},
	{"dvport", CodeEmptyNetworks, "No network data found (dvPort) - network information will be unavailable in inventory"},
	{"vcpu", CodeEmptyCPU, "No CPU detail data found - CPU hot-add and cores-per-socket info will be unavailable"},
	{"vmemory", CodeEmptyMemory, "No memory detail data found - memory hot-add info will be unavailable"},
	{"vdisk", CodeEmptyDisks, "No disk detail data found - individual disk information will be unavailable"},
	{"vnetwork", CodeEmptyNICs, "No NIC detail data found - network adapter information will be unavailable"},
}

// ValidateSchema checks the ingested schema for required tables, columns, and data.
// The table parameter specifies which table to validate VM data against
// (e.g., "vinfo_raw" for RVTools, "vinfo" for SQLite).
// Returns a ValidationResult with errors (fatal) and warnings (non-fatal).
func (p *Parser) ValidateSchema(ctx context.Context, table string) ValidationResult {
	result := ValidationResult{}

	p.validateVinfoData(ctx, &result, table)
	p.validateOptionalTables(ctx, &result)

	return result
}

// validateVinfoData checks the specified table for VM data quality.
// Reports NO_VMS if table has no data rows, otherwise reports specific column errors.
// The table parameter allows validating against "vinfo_raw" (RVTools) or "vinfo" (SQLite).
func (p *Parser) validateVinfoData(ctx context.Context, result *ValidationResult, table string) {
	rowCount, err := p.countRows(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s`, table))
	if err != nil {
		result.Errors = append(result.Errors, ValidationIssue{
			Code:    CodeColumnValidationFailed,
			Table:   table,
			Message: fmt.Sprintf("could not read vInfo table: %v", err),
		})
		return
	}
	if rowCount == 0 {
		result.Errors = append(result.Errors, ValidationIssue{
			Code:    CodeNoVMs,
			Table:   table,
			Message: "No VMs found in vInfo sheet - cannot build inventory without VM data",
		})
		return
	}

	p.appendNonEmptyColumnIssue(ctx, result, table, "VM ID", CodeMissingVMID, "'VM ID' column is missing or empty in the vInfo sheet - VMs cannot be identified")
	p.appendNonEmptyColumnIssue(ctx, result, table, "VM", CodeMissingVMName, "'VM' column is missing or empty in the vInfo sheet - VMs cannot be identified")
	p.appendNonEmptyColumnIssue(ctx, result, table, "Cluster", CodeMissingCluster, "'Cluster' column is missing or empty in the vInfo sheet - cannot determine cluster membership")

	if table == "vinfo_raw" {
		p.appendNonEmptyColumnIssue(ctx, result, table, "VI SDK UUID", CodeMissingVISDKUUID, "The RVTools export is missing the 'VI SDK UUID' column on the vInfo sheet, or all values are empty. That column is required to identify your vCenter. Re-export the file using RVTools so the vInfo sheet includes a non-empty VI SDK UUID for at least one VM.")
	}
}

// appendNonEmptyColumnIssue appends a user-facing error if the column has no non-empty values,
// or COLUMN_VALIDATION_FAILED if the database query fails (so query errors are not mistaken for bad data).
func (p *Parser) appendNonEmptyColumnIssue(ctx context.Context, result *ValidationResult, table, column, code, emptyMsg string) {
	hasData, err := p.hasValidColumnData(ctx, table, column)
	if err != nil {
		result.Errors = append(result.Errors, ValidationIssue{
			Code:    CodeColumnValidationFailed,
			Table:   table,
			Column:  column,
			Message: fmt.Sprintf("could not validate column %q: %v", column, err),
		})
		return
	}
	if !hasData {
		result.Errors = append(result.Errors, ValidationIssue{
			Code:    code,
			Table:   table,
			Column:  column,
			Message: emptyMsg,
		})
	}
}

// hasValidColumnData checks if a table has a column with at least one non-empty value.
// Returns (false, nil) if the column is missing or empty; (false, err) if the query fails.
func (p *Parser) hasValidColumnData(ctx context.Context, table, column string) (bool, error) {
	query := fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE "%s" IS NOT NULL AND TRIM("%s") != ''`, table, column, column)
	n, err := p.countRows(ctx, query)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// validateOptionalTables checks that optional tables have data, producing warnings if empty.
func (p *Parser) validateOptionalTables(ctx context.Context, result *ValidationResult) {
	for _, check := range warningChecks {
		n, err := p.countRows(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s`, check.table))
		if err != nil {
			// Log unexpected errors (connection, permissions); missing tables are expected
			if !isTableMissingError(err) {
				zap.S().Named("duckdb_parser").Warnf("Failed to validate optional table %s: %v", check.table, err)
			}
			continue
		}
		if n == 0 {
			result.Warnings = append(result.Warnings, ValidationIssue{
				Code:    check.code,
				Table:   check.table,
				Message: check.message,
			})
		}
	}
}

func (p *Parser) countRows(ctx context.Context, query string) (int, error) {
	var count int
	if err := p.db.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// isTableMissingError returns true if the error indicates a table doesn't exist.
func isTableMissingError(err error) bool {
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "table") &&
		(strings.Contains(errMsg, "not found") ||
			strings.Contains(errMsg, "does not exist") ||
			strings.Contains(errMsg, "no such table"))
}
