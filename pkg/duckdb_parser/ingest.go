package duckdb_parser

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"go.uber.org/zap"
)

var stmtRegex = regexp.MustCompile(`(?s)(CREATE|INSERT|UPDATE|DROP|ALTER|WITH|INSTALL|LOAD|ATTACH|DETACH|DELETE).*?;`)

// criticalStmtPatterns defines patterns for statements that must succeed.
// If any of these fail, the ingestion should fail immediately.
var criticalStmtPatterns = []string{
	"INSTALL ",           // Extension installation must succeed
	"LOAD ",              // Extension loading must succeed
	"CREATE TABLE vinfo", // Main VM data table creation must succeed
}

// isCriticalStatement checks if a statement matches any critical pattern.
func isCriticalStatement(stmt string) bool {
	upperStmt := strings.ToUpper(stmt)
	for _, pattern := range criticalStmtPatterns {
		if strings.Contains(upperStmt, strings.ToUpper(pattern)) {
			return true
		}
	}
	return false
}

// xlsxErrorMappings maps DuckDB error substrings to user-friendly messages.
var xlsxErrorMappings = map[string]string{
	"No xl/workbook.xml found":         "The file is corrupted or not a valid Excel file",
	"\"vInfo\" not found in xlsx file": "File is not a valid RVTools export (missing required 'vInfo' sheet)",
	"not found in xlsx file":           "Required worksheet not found in the Excel file",
}

// translateXLSXError converts technical DuckDB xlsx errors to user-friendly messages.
func translateXLSXError(err error) error {
	errStr := err.Error()
	for pattern, message := range xlsxErrorMappings {
		if strings.Contains(errStr, pattern) {
			return fmt.Errorf("%s", message)
		}
	}
	return err
}

// ClearData removes all data from tables without dropping the schema.
// This allows re-ingestion of new RVTools data into the same database instance.
func (p *Parser) ClearData() error {
	query, err := p.builder.ClearDataQuery()
	if err != nil {
		return fmt.Errorf("building clear data query: %w", err)
	}
	if err := p.executeStatements(query); err != nil {
		return fmt.Errorf("clearing data: %w", err)
	}
	return nil
}

// IngestRvTools ingests data from an RVTools Excel file, runs VM validation if a validator
// is configured, and validates the schema for required tables/columns.
// Returns a ValidationResult with errors (fatal) and warnings (non-fatal).
// If ValidationResult.HasErrors() is true, the inventory cannot be built.
func (p *Parser) IngestRvTools(ctx context.Context, excelFile string) (ValidationResult, error) {
	query, err := p.builder.IngestRvtoolsQuery(excelFile)
	if err != nil {
		return ValidationResult{}, fmt.Errorf("building rvtools ingestion query: %w", err)
	}
	if err := p.executeStatements(query); err != nil {
		return ValidationResult{}, fmt.Errorf("ingesting rvtools data: %w", err)
	}

	// Validate schema - check for required tables/columns/data
	result := p.ValidateSchema(ctx)

	// Drop vinfo_raw now that validation is complete
	p.dropVinfoRaw()

	// Only run VM validation if schema is valid (we have VMs to validate)
	if result.IsValid() {
		if err := p.validateVMs(ctx); err != nil {
			return result, fmt.Errorf("validating VMs: %w", err)
		}
	}

	return result, nil
}

// IngestSqlite ingests data from a forklift SQLite database, runs VM validation if a validator
// is configured, and validates the schema for required tables/columns.
// Returns a ValidationResult with errors (fatal) and warnings (non-fatal).
// If ValidationResult.HasErrors() is true, the inventory cannot be built.
func (p *Parser) IngestSqlite(ctx context.Context, sqliteFile string) (ValidationResult, error) {
	query, err := p.builder.IngestSqliteQuery(sqliteFile)
	if err != nil {
		return ValidationResult{}, fmt.Errorf("building sqlite ingestion query: %w", err)
	}
	if err := p.executeStatements(query); err != nil {
		return ValidationResult{}, fmt.Errorf("ingesting sqlite data: %w", err)
	}

	// Validate schema - check for required tables/columns/data
	result := p.ValidateSchema(ctx)

	// Only run VM validation if schema is valid (we have VMs to validate)
	if result.IsValid() {
		if err := p.validateVMs(ctx); err != nil {
			return result, fmt.Errorf("validating VMs: %w", err)
		}
	}

	return result, nil
}

// dropVinfoRaw drops the temporary vinfo_raw table used during RVTools ingestion.
// This table holds unfiltered data from the Excel file and is only needed for validation.
func (p *Parser) dropVinfoRaw() {
	_, _ = p.db.Exec("DROP TABLE IF EXISTS vinfo_raw")
}

// executeStatements executes a multi-statement SQL string.
// Critical statements (INSTALL, LOAD, CREATE TABLE vinfo) must succeed or an error is returned.
// Non-critical statements (INSERT for optional sheets, ALTER for optional columns) are logged but don't fail.
func (p *Parser) executeStatements(query string) error {
	stmts := stmtRegex.FindAllString(query, -1)
	for _, stmt := range stmts {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := p.db.Exec(stmt); err != nil {
			if isCriticalStatement(stmt) {
				return translateXLSXError(err)
			}
			// Non-critical failures are logged but don't stop execution
			zap.S().Debugw("non-critical statement failed", "error", err)
		}
	}
	return nil
}

// validateVMs runs the configured VM validator (e.g., OPA) to populate the concerns table.
func (p *Parser) validateVMs(ctx context.Context) error {
	if p.validator == nil {
		return nil
	}

	vms, err := p.VMs(ctx, Filters{}, Options{})
	if err != nil {
		return fmt.Errorf("getting VMs for validation: %w", err)
	}

	builder := NewConcernValuesBuilder()
	for _, vm := range vms {
		concerns, err := p.validator.Validate(ctx, vm)
		if err != nil {
			zap.S().Warnw("validation failed for VM", "vm_id", vm.ID, "error", err)
			continue
		}
		builder.Append(vm.ID, concerns...)
	}

	return InsertConcerns(ctx, p.db, builder)
}
