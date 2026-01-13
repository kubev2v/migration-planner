package duckdb_parser

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"go.uber.org/zap"
)

var stmtRegex = regexp.MustCompile(`(?s)(CREATE|INSERT|UPDATE|DROP|ALTER|WITH|INSTALL|LOAD|ATTACH|DETACH).*?;`)

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

// executeStatements executes a multi-statement SQL string.
// Errors are logged but not returned since missing sheets are expected in RVTools exports.
func (p *Parser) executeStatements(query string) error {
	stmts := stmtRegex.FindAllString(query, -1)
	for _, stmt := range stmts {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := p.db.Exec(stmt); err != nil {
			zap.S().Debugw("statement failed", "error", err)
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
