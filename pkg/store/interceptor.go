package store

import (
	"context"
	"database/sql"

	"go.uber.org/zap"
)

// QueryInterceptor provides database query methods with transaction awareness.
// Implementations route queries through an active transaction if present in context.
//
// NOTE: DuckDB durability is handled by the WAL — it replays on startup. Explicit
// checkpointing is only needed before file-level operations (close, clone) and is
// available via Store2.Checkpoint().
type QueryInterceptor interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type queryInterceptor struct {
	db     *sql.DB
	logger *zap.SugaredLogger
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
	q.logger.Debugw("exec", "query", query, "args", args)

	tx, ok := q.txFromContext(ctx)
	if ok {
		return tx.ExecContext(ctx, query, args...)
	}

	return q.db.ExecContext(ctx, query, args...)
}

func (q *queryInterceptor) txFromContext(ctx context.Context) (*sql.Tx, bool) {
	tx, ok := ctx.Value(txKey).(*sql.Tx)
	return tx, ok
}
