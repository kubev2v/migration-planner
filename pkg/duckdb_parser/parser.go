package duckdb_parser

import (
	"context"
	"fmt"
	"sync"

	"github.com/kubev2v/migration-planner/pkg/duckdb_parser/models"
	"github.com/kubev2v/migration-planner/pkg/store"
)

// Validator interface for OPA validation.
// Returns slice of concerns (a VM can have multiple).
type Validator interface {
	Validate(ctx context.Context, vm models.VM) ([]models.Concern, error)
}

// Parser provides methods for parsing and querying VMware inventory data.
type Parser struct {
	db              store.QueryInterceptor
	builder         *QueryBuilder
	validator       Validator
	datastoreMu     sync.Mutex
	datastoreSchema *datastoreColumns
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

	p.datastoreMu.Lock()
	p.datastoreSchema = &datastoreColumns{arrays: true, container: true}
	p.datastoreMu.Unlock()
	return nil
}

func (p *Parser) Cleanup() error {
	q, err := p.builder.CleanupQuery()
	if err != nil {
		return fmt.Errorf("building cleanup query: %w", err)
	}
	_, err = p.db.ExecContext(context.Background(), q)
	if err != nil {
		return fmt.Errorf("cleaning up tables: %w", err)
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

type datastoreColumns struct {
	arrays    bool
	container bool
}

func (p *Parser) datastoreStorageColumns(ctx context.Context) (datastoreColumns, error) {
	p.datastoreMu.Lock()
	defer p.datastoreMu.Unlock()

	if p.datastoreSchema != nil {
		return *p.datastoreSchema, nil
	}

	// Read-only historical collections cannot run Init; resolve their projection once.
	rows, err := p.db.QueryContext(ctx, `SELECT column_name FROM information_schema.columns
        WHERE table_catalog = current_database() AND table_schema = 'main'
            AND table_name = 'vdatastore' AND column_name IN ('Storage Arrays', 'Storage Container ID')`)
	if err != nil {
		return datastoreColumns{}, fmt.Errorf("reading datastore schema: %w", err)
	}
	defer func() { _ = rows.Close() }()

	columns := datastoreColumns{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return datastoreColumns{}, fmt.Errorf("scanning datastore schema: %w", err)
		}
		switch name {
		case "Storage Arrays":
			columns.arrays = true
		case "Storage Container ID":
			columns.container = true
		}
	}
	if err := rows.Err(); err != nil {
		return datastoreColumns{}, fmt.Errorf("reading datastore schema: %w", err)
	}
	p.datastoreSchema = &columns
	return columns, nil
}
