package duckdb_parser

import (
	"context"
	"fmt"
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
	CodeNoVMs          = "NO_VMS"
	CodeMissingVMID    = "MISSING_VM_ID"
	CodeMissingVMName  = "MISSING_VM_NAME"
	CodeMissingCluster = "MISSING_CLUSTER"
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
	// First check if we have any rows at all (independent of column existence)
	if p.countRows(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s`, table)) == 0 {
		result.Errors = append(result.Errors, ValidationIssue{
			Code:    CodeNoVMs,
			Table:   table,
			Message: "No VMs found in vInfo sheet - cannot build inventory without VM data",
		})
		return
	}

	// Rows exist — check each required column independently.
	// hasValidColumnData returns false if column is missing OR has no valid values.
	if !p.hasValidColumnData(ctx, table, "VM ID") {
		result.Errors = append(result.Errors, ValidationIssue{
			Code:    CodeMissingVMID,
			Table:   table,
			Column:  "VM ID",
			Message: "'VM ID' column is missing or empty in the vInfo sheet - VMs cannot be identified",
		})
	}

	if !p.hasValidColumnData(ctx, table, "VM") {
		result.Errors = append(result.Errors, ValidationIssue{
			Code:    CodeMissingVMName,
			Table:   table,
			Column:  "VM",
			Message: "'VM' column is missing or empty in the vInfo sheet - VMs cannot be identified",
		})
	}

	if !p.hasValidColumnData(ctx, table, "Cluster") {
		result.Errors = append(result.Errors, ValidationIssue{
			Code:    CodeMissingCluster,
			Table:   table,
			Column:  "Cluster",
			Message: "'Cluster' column is missing or empty in the vInfo sheet - cannot determine cluster membership",
		})
	}
}

// hasValidColumnData checks if a table has a column with at least one non-empty value.
// Returns false if the column doesn't exist or has no valid values.
func (p *Parser) hasValidColumnData(ctx context.Context, table, column string) bool {
	query := fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE "%s" IS NOT NULL AND TRIM("%s") != ''`, table, column, column)
	return p.countRows(ctx, query) > 0
}

// validateOptionalTables checks that optional tables have data, producing warnings if empty.
func (p *Parser) validateOptionalTables(ctx context.Context, result *ValidationResult) {
	for _, check := range warningChecks {
		if p.countRows(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s`, check.table)) == 0 {
			result.Warnings = append(result.Warnings, ValidationIssue{
				Code:    check.code,
				Table:   check.table,
				Message: check.message,
			})
		}
	}
}

// countRows executes a COUNT query and returns the result, or 0 on any error.
func (p *Parser) countRows(ctx context.Context, query string) int {
	var count int
	if err := p.db.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return 0
	}
	return count
}
