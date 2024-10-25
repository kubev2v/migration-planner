package store

import (
	"context"
	"errors"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type contextKey int

const (
	transactionKey contextKey = iota
)

type Tx struct {
	txId int64
	tx   *gorm.DB
	log  logrus.FieldLogger
}

func Commit(ctx context.Context) (context.Context, error) {
	tx, ok := ctx.Value(transactionKey).(*Tx)
	if !ok {
		return ctx, nil
	}

	newCtx := context.WithValue(ctx, transactionKey, nil)
	return newCtx, tx.Commit()
}

func Rollback(ctx context.Context) (context.Context, error) {
	tx, ok := ctx.Value(transactionKey).(*Tx)
	if !ok {
		return ctx, nil
	}

	newCtx := context.WithValue(ctx, transactionKey, nil)
	return newCtx, tx.Rollback()
}

func FromContext(ctx context.Context) *gorm.DB {
	if tx, found := ctx.Value(transactionKey).(*Tx); found {
		if dbTx, err := tx.Db(); err == nil {
			return dbTx
		}
	}
	return nil
}

func newTransactionContext(ctx context.Context, db *gorm.DB, log logrus.FieldLogger) (context.Context, error) {
	//look into the context to see if we have another tx
	_, found := ctx.Value(transactionKey).(*Tx)
	if found {
		return ctx, nil
	}

	// create a new session
	conn := db.Session(&gorm.Session{
		Context: ctx,
	})

	tx, err := newTransaction(conn, log)
	if err != nil {
		return ctx, err
	}

	ctx = context.WithValue(ctx, transactionKey, tx)
	return ctx, nil
}

func newTransaction(db *gorm.DB, log logrus.FieldLogger) (*Tx, error) {
	// must call begin on 'db', which is Gorm.
	tx := db.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}

	// current transaction ID set by postgres.  these are *not* distinct across time
	// and do get reset after postgres performs "vacuuming" to reclaim used IDs.
	var txid struct{ ID int64 }
	tx.Raw("select txid_current() as id").Scan(&txid)

	return &Tx{
		txId: txid.ID,
		tx:   tx,
		log:  log,
	}, nil
}

func (t *Tx) Db() (*gorm.DB, error) {
	if t.tx != nil {
		return t.tx, nil
	}
	return nil, errors.New("transaction hasn't started yet")
}

func (t *Tx) Commit() error {
	if t.tx == nil {
		return errors.New("transaction hasn't started yet")
	}

	if err := t.tx.Commit().Error; err != nil {
		t.log.Errorf("failed to commit transaction %d: %w", t.txId, err)
		return err
	}
	t.log.Debugf("transaction %d commited", t.txId)
	t.tx = nil // in case we call commit twice
	return nil
}

func (t *Tx) Rollback() error {
	if t.tx == nil {
		return errors.New("transaction hasn't started yet")
	}

	if err := t.tx.Rollback().Error; err != nil {
		t.log.Errorf("failed to rollback transaction %d: %w", t.txId, err)
		return err
	}
	t.tx = nil // in case we call commit twice

	t.log.Debugf("transaction %d rollback", t.txId)
	return nil
}
