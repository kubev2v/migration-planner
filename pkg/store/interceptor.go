package store

import (
	"context"
	"database/sql"
	"sync"

	"go.uber.org/zap"
)

// QueryInterceptor provides database query methods with transaction awareness.
// Implementations route queries through an active transaction if present in context.
//
// NOTE: The default implementation (queryInterceptor) is DuckDB-specific. It uses:
// - Mutex serialization in ExecContext to satisfy DuckDB's single-connection constraint
// - FORCE CHECKPOINT after non-transactional writes to flush DuckDB's WAL to the main file
//
// If supporting other databases, a separate implementation would be needed without
// these DuckDB-specific behaviors.
type QueryInterceptor interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type queryInterceptor struct {
	db     *sql.DB
	logger *zap.SugaredLogger
	mu     sync.Mutex
}

func NewQueryInterceptor(db *sql.DB) QueryInterceptor {
	return &queryInterceptor{
		db:     db,
		logger: zap.S().Named("store"),
	}
}

func (q *queryInterceptor) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	q.logger.Debugw("query_row", "query", query, "args", args)

	tx, ok := q.txFromContext(ctx)
	if ok {
		return tx.QueryRowContext(ctx, query, args...)
	}

	return q.db.QueryRowContext(ctx, query, args...)
}

func (q *queryInterceptor) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	q.logger.Debugw("query", "query", query, "args", args)

	tx, ok := q.txFromContext(ctx)
	if ok {
		return tx.QueryContext(ctx, query, args...)
	}

	return q.db.QueryContext(ctx, query, args...)
}

func (q *queryInterceptor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	// Serialize all exec calls to satisfy DuckDB's single-connection constraint.
	// DuckDB only allows one write operation at a time on a single connection.
	// Without this mutex, concurrent ExecContext calls would fail with "database is locked" errors.
	// This represents a concurrency bottleneck but is required for DuckDB correctness.
	q.mu.Lock()
	defer q.mu.Unlock()

	q.logger.Debugw("exec", "query", query, "args", args)

	tx, ok := q.txFromContext(ctx)
	if ok {
		return tx.ExecContext(ctx, query, args...)
	}

	result, err := q.db.ExecContext(ctx, query, args...)
	if err != nil {
		return result, err
	}

	// Issue a DuckDB-specific FORCE CHECKPOINT to flush the write-ahead log (WAL) to the main database file.
	// This ensures durability for non-transactional writes by persisting changes immediately.
	// FORCE CHECKPOINT is DuckDB-specific SQL and would fail on other databases.
	// We only checkpoint on the non-transaction path; transactions handle their own durability on commit.
	if _, cpErr := q.db.ExecContext(ctx, "FORCE CHECKPOINT"); cpErr != nil {
		q.logger.Warnw("checkpoint failed", "error", cpErr)
	}
	return result, nil
}

func (q *queryInterceptor) txFromContext(ctx context.Context) (*sql.Tx, bool) {
	tx, ok := ctx.Value(txKey).(*sql.Tx)
	return tx, ok
}
