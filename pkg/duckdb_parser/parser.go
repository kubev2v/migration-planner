package duckdb_parser

import (
	"context"
	"fmt"
	"time"

	"github.com/kubev2v/migration-planner/pkg/duckdb_parser/models"
	"github.com/kubev2v/migration-planner/pkg/store"
)

// Validator interface for OPA validation.
// Returns slice of concerns (a VM can have multiple).
type Validator interface {
	Validate(ctx context.Context, vm models.VM) ([]models.Concern, error)
}

// Parser provides methods for parsing and querying VMware inventory data.
// Ingestion and inventory building on the same Parser must run sequentially.
type Parser struct {
	db              store.QueryInterceptor
	builder         *QueryBuilder
	validator       Validator
	collectionTimes []*time.Time
}

// New creates a new Parser with optional validator.
func New(db store.QueryInterceptor, validator Validator) *Parser {
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
	// Use background context for schema creation (not transactional)
	_, err = p.db.ExecContext(context.Background(), q)
	return err
}
