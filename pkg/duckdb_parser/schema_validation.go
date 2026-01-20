package duckdb_parser

import (
	"context"
	"database/sql"
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
	Code    string // Machine-readable code (e.g., "MISSING_VMS", "EMPTY_HOSTS")
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
	CodeNoVMs         = "NO_VMS"
	CodeMissingVMID   = "MISSING_VM_ID"
	CodeMissingVMName = "MISSING_VM_NAME"
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

// tableCheck defines a validation check for a table.
type tableCheck struct {
	table       string
	code        string
	message     string
	isError     bool   // true = error, false = warning
	minRows     int    // minimum required rows (0 means just check existence)
	columnCheck string // optional: column to check for non-null values
}

// schemaChecks defines all validation checks to perform.
var schemaChecks = []tableCheck{
	// Errors - these prevent inventory from being built
	{
		table:   "vinfo",
		code:    CodeNoVMs,
		message: "No VMs found in vinfo table - cannot build inventory without VM data",
		isError: true,
		minRows: 1,
	},
	{
		table:       "vinfo",
		code:        CodeMissingVMID,
		message:     "VM ID column has no valid values - VMs cannot be identified",
		isError:     true,
		columnCheck: "VM ID",
	},
	{
		table:       "vinfo",
		code:        CodeMissingVMName,
		message:     "VM name column has no valid values - VMs cannot be identified",
		isError:     true,
		columnCheck: "VM",
	},

	// Warnings - inventory can be built but with missing data
	{
		table:   "vhost",
		code:    CodeEmptyHosts,
		message: "No host data found - host information will be unavailable in inventory",
		isError: false,
		minRows: 1,
	},
	{
		table:   "vdatastore",
		code:    CodeEmptyDatastores,
		message: "No datastore data found - storage information will be unavailable in inventory",
		isError: false,
		minRows: 1,
	},
	{
		table:   "dvport",
		code:    CodeEmptyNetworks,
		message: "No network data found (dvPort) - network information will be unavailable in inventory",
		isError: false,
		minRows: 1,
	},
	{
		table:   "vcpu",
		code:    CodeEmptyCPU,
		message: "No CPU detail data found - CPU hot-add and cores-per-socket info will be unavailable",
		isError: false,
		minRows: 1,
	},
	{
		table:   "vmemory",
		code:    CodeEmptyMemory,
		message: "No memory detail data found - memory hot-add info will be unavailable",
		isError: false,
		minRows: 1,
	},
	{
		table:   "vdisk",
		code:    CodeEmptyDisks,
		message: "No disk detail data found - individual disk information will be unavailable",
		isError: false,
		minRows: 1,
	},
	{
		table:   "vnetwork",
		code:    CodeEmptyNICs,
		message: "No NIC detail data found - network adapter information will be unavailable",
		isError: false,
		minRows: 1,
	},
}

// ValidateSchema checks the ingested schema for required tables, columns, and data.
// It returns a ValidationResult with errors (fatal) and warnings (non-fatal).
func (p *Parser) ValidateSchema(ctx context.Context) ValidationResult {
	result := ValidationResult{}

	for _, check := range schemaChecks {
		issue := p.runCheck(ctx, check)
		if issue != nil {
			if check.isError {
				result.Errors = append(result.Errors, *issue)
			} else {
				result.Warnings = append(result.Warnings, *issue)
			}
		}
	}

	return result
}

// runCheck executes a single validation check and returns an issue if the check fails.
func (p *Parser) runCheck(ctx context.Context, check tableCheck) *ValidationIssue {
	var count int
	var err error

	if check.columnCheck != "" {
		// Check for non-null, non-empty values in a specific column
		query := fmt.Sprintf(
			`SELECT COUNT(*) FROM %s WHERE "%s" IS NOT NULL AND TRIM("%s") != ''`,
			check.table, check.columnCheck, check.columnCheck,
		)
		err = p.db.QueryRowContext(ctx, query).Scan(&count)
	} else {
		// Check row count
		query := fmt.Sprintf(`SELECT COUNT(*) FROM %s`, check.table)
		err = p.db.QueryRowContext(ctx, query).Scan(&count)
	}

	if err != nil {
		// If table doesn't exist or query fails, treat as 0 rows
		if err == sql.ErrNoRows {
			count = 0
		} else {
			// Log error but continue - table might not exist
			count = 0
		}
	}

	// Check if validation passes
	if count >= check.minRows && (check.columnCheck == "" || count > 0) {
		return nil // Check passed
	}

	return &ValidationIssue{
		Code:    check.code,
		Table:   check.table,
		Column:  check.columnCheck,
		Message: check.message,
	}
}
