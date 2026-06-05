package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "github.com/marcboeker/go-duckdb/v2"
)

func TestTransactor_BasicCommit(t *testing.T) {
	db := setupTestDB(t)
	transactor := NewTransactor(db)

	if _, err := db.Exec("CREATE TABLE test (id INTEGER, name VARCHAR)"); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	err := transactor.WithTx(context.Background(), func(txCtx context.Context) error {
		tx := txCtx.Value(txKey).(*sql.Tx)
		_, err := tx.Exec("INSERT INTO test VALUES (1, 'committed')")
		return err
	})

	if err != nil {
		t.Fatalf("transaction failed: %v", err)
	}

	var name string
	if err := db.QueryRow("SELECT name FROM test WHERE id = 1").Scan(&name); err != nil {
		t.Fatalf("failed to query after commit: %v", err)
	}

	if name != "committed" {
		t.Errorf("expected 'committed', got '%s'", name)
	}
}

func TestTransactor_Rollback(t *testing.T) {
	db := setupTestDB(t)
	transactor := NewTransactor(db)

	if _, err := db.Exec("CREATE TABLE test (id INTEGER, name VARCHAR)"); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	expectedErr := errors.New("intentional error")
	err := transactor.WithTx(context.Background(), func(txCtx context.Context) error {
		tx := txCtx.Value(txKey).(*sql.Tx)
		if _, err := tx.Exec("INSERT INTO test VALUES (1, 'should_rollback')"); err != nil {
			return err
		}
		return expectedErr
	})

	if err != expectedErr {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM test").Scan(&count); err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0 rows after rollback, got %d", count)
	}
}

func TestTransactor_NestedTransactionBlocked(t *testing.T) {
	db := setupTestDB(t)
	transactor := NewTransactor(db)

	err := transactor.WithTx(context.Background(), func(outerCtx context.Context) error {
		return transactor.WithTx(outerCtx, func(innerCtx context.Context) error {
			return nil
		})
	})

	if err == nil || err.Error() != "nested transactions not supported" {
		t.Errorf("expected 'nested transactions not supported' error, got %v", err)
	}
}

func TestTransactor_PanicRollback(t *testing.T) {
	db := setupTestDB(t)
	transactor := NewTransactor(db)

	if _, err := db.Exec("CREATE TABLE test (id INTEGER, name VARCHAR)"); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic to propagate")
		}
	}()

	_ = transactor.WithTx(context.Background(), func(txCtx context.Context) error {
		tx := txCtx.Value(txKey).(*sql.Tx)
		if _, err := tx.Exec("INSERT INTO test VALUES (1, 'panic')"); err != nil {
			return err
		}
		panic("intentional panic")
	})
}

func TestTransactor_PanicRollbackVerifyNoData(t *testing.T) {
	db := setupTestDB(t)
	transactor := NewTransactor(db)

	if _, err := db.Exec("CREATE TABLE test (id INTEGER, name VARCHAR)"); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	func() {
		defer func() {
			_ = recover()
		}()

		_ = transactor.WithTx(context.Background(), func(txCtx context.Context) error {
			tx := txCtx.Value(txKey).(*sql.Tx)
			if _, err := tx.Exec("INSERT INTO test VALUES (1, 'panic')"); err != nil {
				return err
			}
			panic("intentional panic")
		})
	}()

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM test").Scan(&count); err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0 rows after panic rollback, got %d", count)
	}
}

func TestTransactor_CommitError(t *testing.T) {
	db := setupTestDB(t)
	transactor := NewTransactor(db)

	if _, err := db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	if _, err := db.Exec("INSERT INTO test VALUES (1)"); err != nil {
		t.Fatalf("failed to insert initial row: %v", err)
	}

	err := transactor.WithTx(context.Background(), func(txCtx context.Context) error {
		tx := txCtx.Value(txKey).(*sql.Tx)
		_, err := tx.Exec("INSERT INTO test VALUES (1)")
		return err
	})

	if err == nil {
		t.Error("expected commit error due to constraint violation")
	}
}
