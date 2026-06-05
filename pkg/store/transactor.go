package store

import (
	"context"
	"database/sql"
	"errors"
)

// txKey is a private key for storing transactions in context.
// Uses unexported type to prevent collisions with other packages.
type contextKey int

const txKey contextKey = 0

// Transactor provides transaction management.
type Transactor interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type dbTransactor struct {
	db *sql.DB
}

func NewTransactor(db *sql.DB) Transactor {
	return &dbTransactor{db: db}
}

func (t *dbTransactor) WithTx(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if ctx.Value(txKey) != nil {
		return errors.New("nested transactions not supported")
	}

	tx, err := t.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}

	var committed bool
	defer func() {
		if p := recover(); p != nil {
			// On panic, attempt rollback (unless we already committed)
			if !committed {
				if rbErr := tx.Rollback(); rbErr != nil {
					err = errors.Join(errors.New("panic during transaction"), rbErr)
				} else {
					err = errors.New("panic during transaction")
				}
			}
			panic(p)
		} else if err != nil && !committed {
			// On error return from fn, rollback (unless we already committed).
			// Skip rollback if commit was attempted to avoid confusing error messages
			// like "primary key violation" + "sql: transaction has already been committed or rolled back".
			if rbErr := tx.Rollback(); rbErr != nil {
				err = errors.Join(err, rbErr)
			}
		}
	}()

	txContext := context.WithValue(ctx, txKey, tx)

	if err = fn(txContext); err != nil {
		return err
	}

	committed = true
	if err = tx.Commit(); err != nil {
		return err
	}

	return nil
}
