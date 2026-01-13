package duckdb_parser

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/kubev2v/migration-planner/pkg/duckdb_parser/models"
)

// ConcernValuesBuilder builds SQL VALUES for bulk inserting concerns.
type ConcernValuesBuilder struct {
	values []string
}

// NewConcernValuesBuilder creates a new ConcernValuesBuilder.
func NewConcernValuesBuilder() *ConcernValuesBuilder {
	return &ConcernValuesBuilder{}
}

// Append adds concerns for a VM to the builder.
func (cb *ConcernValuesBuilder) Append(vmID string, concerns ...models.Concern) *ConcernValuesBuilder {
	for _, c := range concerns {
		value := fmt.Sprintf("('%s', '%s', '%s', '%s', '%s')",
			escapeSQLString(vmID),
			escapeSQLString(c.Id),
			escapeSQLString(c.Label),
			escapeSQLString(c.Category),
			escapeSQLString(c.Assessment),
		)
		cb.values = append(cb.values, value)
	}
	return cb
}

// Build returns the VALUES clause string, or empty string if no values.
func (cb *ConcernValuesBuilder) Build() string {
	if len(cb.values) == 0 {
		return ""
	}
	return strings.Join(cb.values, ", ")
}

// IsEmpty returns true if no concerns have been added.
func (cb *ConcernValuesBuilder) IsEmpty() bool {
	return len(cb.values) == 0
}

// InsertConcerns executes the bulk insert of concerns into the database.
func InsertConcerns(ctx context.Context, db *sql.DB, builder *ConcernValuesBuilder) error {
	valuesStr := builder.Build()
	if valuesStr == "" {
		return nil
	}

	query := fmt.Sprintf(
		`INSERT INTO concerns ("VM_ID", "Concern_ID", "Label", "Category", "Assessment") VALUES %s;`,
		valuesStr,
	)

	if _, err := db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("inserting concerns: %w", err)
	}

	return nil
}

// escapeSQLString escapes single quotes for SQL string literals.
func escapeSQLString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
