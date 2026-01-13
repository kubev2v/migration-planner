package duckdb_parser

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kubev2v/migration-planner/pkg/duckdb_parser/models"
)

// Validator interface for OPA validation.
// Returns slice of concerns (a VM can have multiple).
type Validator interface {
	Validate(ctx context.Context, vm models.VM) ([]models.Concern, error)
}

// Parser provides methods for parsing and querying VMware inventory data.
type Parser struct {
	db        *sql.DB
	builder   *QueryBuilder
	validator Validator
}

// New creates a new Parser with optional validator.
func New(db *sql.DB, validator Validator) *Parser {
	return &Parser{
		db:        db,
		builder:   NewBuilder(),
		validator: validator,
	}
}

// Init creates the database schema.
func (p *Parser) Init() error {
	if err := p.createSchema(); err != nil {
		return fmt.Errorf("creating schema: %w", err)
	}
	return nil
}

func (p *Parser) createSchema() error {
	q, err := p.builder.CreateSchemaQuery()
	if err != nil {
		return err
	}
	_, err = p.db.Exec(q)
	return err
}
