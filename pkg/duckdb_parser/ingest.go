package duckdb_parser

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"
)

var stmtRegex = regexp.MustCompile(`(?s)(CREATE|INSERT|UPDATE|DROP|ALTER|WITH|INSTALL|LOAD|ATTACH|DETACH|DELETE).*?;`)

// criticalStmtPatterns defines patterns for statements that must succeed.
// If any of these fail, the ingestion should fail immediately.
var criticalStmtPatterns = []string{
	"INSTALL ",           // Extension installation must succeed
	"LOAD ",              // Extension loading must succeed
	"CREATE TABLE vinfo", // Main VM data table creation must succeed
	"WITH metadata AS (", // SQLite VM and metadata insert must succeed
	"UPDATE vinfo_raw SET metadata",
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

// IngestRvTools ingests data from an RVTools Excel file, runs VM validation if a validator
// is configured, and validates the schema for required tables/columns.
// Returns a ValidationResult with errors (fatal) and warnings (non-fatal).
// If ValidationResult.HasErrors() is true, the inventory cannot be built.
func (p *Parser) IngestRvTools(ctx context.Context, excelFile string) (result ValidationResult, err error) {
	metadata, err := readRVToolsMetadata(ctx, excelFile)
	if err != nil {
		return ValidationResult{}, fmt.Errorf("reading RVTools metadata: %w", err)
	}
	var args []any
	if len(metadata) > 0 {
		data, err := json.Marshal(metadata)
		if err != nil {
			return ValidationResult{}, fmt.Errorf("encoding RVTools metadata: %w", err)
		}
		args = []any{string(data)}
	}
	query, err := p.builder.IngestRvtoolsQuery(excelFile, len(metadata) > 0)
	if err != nil {
		return ValidationResult{}, fmt.Errorf("building rvtools ingestion query: %w", err)
	}
	defer func() {
		if err == nil {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if cleanupErr := p.dropVinfoRaw(cleanupCtx); cleanupErr != nil {
			zap.S().Warnw("dropping vinfo_raw after failed import", "error", cleanupErr)
		}
	}()
	if err := p.executeStatements(ctx, query, args...); err != nil {
		return ValidationResult{}, fmt.Errorf("ingesting rvtools data: %w", err)
	}

	// Validate schema against vinfo_raw (unfiltered RVTools data) for granular error reporting
	result = p.ValidateSchema(ctx, "vinfo_raw")

	// Drop vinfo_raw now that validation is complete
	if err := p.dropVinfoRaw(ctx); err != nil {
		return result, fmt.Errorf("dropping vinfo_raw: %w", err)
	}

	// Only run post-ingestion steps if schema is valid (we have VMs to process)
	if result.IsValid() {
		if err := p.populateComplexity(ctx); err != nil {
			return result, fmt.Errorf("populating complexity: %w", err)
		}
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
// Optional category names come from temp.main.tag_categories on the same connection.
func (p *Parser) IngestSqlite(ctx context.Context, sqliteFile string) (result ValidationResult, err error) {
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, cleanupErr := p.db.ExecContext(cleanupCtx, "DROP TABLE IF EXISTS temp.main.tag_categories"); cleanupErr != nil {
			zap.S().Warnw("dropping tag categories after import", "error", cleanupErr)
		}
		if err != nil {
			if _, cleanupErr := p.db.ExecContext(cleanupCtx, "DETACH DATABASE IF EXISTS src"); cleanupErr != nil {
				zap.S().Warnw("detaching SQLite source after failed import", "error", cleanupErr)
			}
		}
	}()
	if _, err := p.db.ExecContext(ctx, "CREATE TEMP TABLE IF NOT EXISTS tag_categories (id VARCHAR, name VARCHAR)"); err != nil {
		return ValidationResult{}, fmt.Errorf("creating tag categories: %w", err)
	}
	query, err := p.builder.IngestSqliteQuery(sqliteFile)
	if err != nil {
		return ValidationResult{}, fmt.Errorf("building sqlite ingestion query: %w", err)
	}
	if err := p.executeStatements(ctx, query); err != nil {
		return ValidationResult{}, fmt.Errorf("ingesting sqlite data: %w", err)
	}

	// Validate schema against vinfo (SQLite inserts directly into vinfo, no vinfo_raw)
	result = p.ValidateSchema(ctx, "vinfo")

	// Only run post-ingestion steps if schema is valid (we have VMs to process)
	if result.IsValid() {
		if err := p.populateComplexity(ctx); err != nil {
			return result, fmt.Errorf("populating complexity: %w", err)
		}
		if err := p.validateVMs(ctx); err != nil {
			return result, fmt.Errorf("validating VMs: %w", err)
		}
	}

	return result, nil
}

// dropVinfoRaw drops the temporary vinfo_raw table used during RVTools ingestion.
// This table holds unfiltered data from the Excel file and is only needed for validation.
func (p *Parser) dropVinfoRaw(ctx context.Context) error {
	_, err := p.db.ExecContext(ctx, "DROP TABLE IF EXISTS vinfo_raw")
	return err
}

// executeStatements executes a multi-statement SQL string.
// Critical statements must succeed or an error is returned.
// Non-critical statements (INSERT for optional sheets, ALTER for optional columns) are logged but don't fail.
func (p *Parser) executeStatements(ctx context.Context, query string, args ...any) error {
	stmts := stmtRegex.FindAllString(query, -1)
	for _, stmt := range stmts {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		var stmtArgs []any
		if strings.HasPrefix(stmt, "UPDATE vinfo_raw SET metadata") {
			stmtArgs = args
		}
		if _, err := p.db.ExecContext(ctx, stmt, stmtArgs...); err != nil {
			if isCriticalStatement(stmt) {
				return translateXLSXError(err)
			}
			// Non-critical failures are logged but don't stop execution
			zap.S().Debugw("non-critical statement failed", "error", err)
		}
	}
	return nil
}

// populateComplexity computes and stores per-VM migration complexity based on OS type and disk size.
func (p *Parser) populateComplexity(ctx context.Context) error {
	query, err := p.builder.PopulateComplexityQuery()
	if err != nil {
		return fmt.Errorf("building complexity query: %w", err)
	}
	if _, err := p.db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("executing complexity query: %w", err)
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
