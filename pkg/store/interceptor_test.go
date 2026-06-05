package store

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/marcboeker/go-duckdb/v2"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close database: %v", err)
		}
	})
	return db
}

func TestQueryInterceptor_DirectDBAccess(t *testing.T) {
	db := setupTestDB(t)
	qi := NewQueryInterceptor(db)

	ctx := context.Background()

	if _, err := qi.ExecContext(ctx, "CREATE TABLE test (id INTEGER, name VARCHAR)"); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	if _, err := qi.ExecContext(ctx, "INSERT INTO test VALUES (1, 'foo')"); err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	row := qi.QueryRowContext(ctx, "SELECT name FROM test WHERE id = ?", 1)
	var name string
	if err := row.Scan(&name); err != nil {
		t.Fatalf("failed to query row: %v", err)
	}

	if name != "foo" {
		t.Errorf("expected 'foo', got '%s'", name)
	}
}

func TestQueryInterceptor_TransactionRouting(t *testing.T) {
	db := setupTestDB(t)
	qi := NewQueryInterceptor(db)
	transactor := NewTransactor(db)

	if _, err := qi.ExecContext(context.Background(), "CREATE TABLE test (id INTEGER, name VARCHAR)"); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	err := transactor.WithTx(context.Background(), func(txCtx context.Context) error {
		if _, err := qi.ExecContext(txCtx, "INSERT INTO test VALUES (1, 'inside_tx')"); err != nil {
			return err
		}

		row := qi.QueryRowContext(txCtx, "SELECT name FROM test WHERE id = ?", 1)
		var name string
		if err := row.Scan(&name); err != nil {
			return err
		}

		if name != "inside_tx" {
			t.Errorf("expected 'inside_tx', got '%s'", name)
		}

		return nil
	})

	if err != nil {
		t.Fatalf("transaction failed: %v", err)
	}

	row := qi.QueryRowContext(context.Background(), "SELECT name FROM test WHERE id = ?", 1)
	var name string
	if err := row.Scan(&name); err != nil {
		t.Fatalf("failed to query after commit: %v", err)
	}

	if name != "inside_tx" {
		t.Errorf("expected committed value 'inside_tx', got '%s'", name)
	}
}

func TestQueryInterceptor_TransactionRollback(t *testing.T) {
	db := setupTestDB(t)
	qi := NewQueryInterceptor(db)
	transactor := NewTransactor(db)

	if _, err := qi.ExecContext(context.Background(), "CREATE TABLE test (id INTEGER, name VARCHAR)"); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	expectedErr := sql.ErrNoRows
	err := transactor.WithTx(context.Background(), func(txCtx context.Context) error {
		if _, err := qi.ExecContext(txCtx, "INSERT INTO test VALUES (1, 'should_rollback')"); err != nil {
			return err
		}
		return expectedErr
	})

	if err != expectedErr {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}

	row := qi.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM test")
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0 rows after rollback, got %d", count)
	}
}

func TestQueryInterceptor_DirectExecSucceeds(t *testing.T) {
	db := setupTestDB(t)
	qi := NewQueryInterceptor(db)

	if _, err := qi.ExecContext(context.Background(), "CREATE TABLE test (id INTEGER)"); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	if _, err := qi.ExecContext(context.Background(), "INSERT INTO test VALUES (1)"); err != nil {
		t.Fatalf("failed to insert: %v", err)
	}
}

func TestQueryInterceptor_TransactionExecSucceeds(t *testing.T) {
	db := setupTestDB(t)
	qi := NewQueryInterceptor(db)
	transactor := NewTransactor(db)

	if _, err := qi.ExecContext(context.Background(), "CREATE TABLE test (id INTEGER)"); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	err := transactor.WithTx(context.Background(), func(txCtx context.Context) error {
		_, err := qi.ExecContext(txCtx, "INSERT INTO test VALUES (1)")
		return err
	})

	if err != nil {
		t.Fatalf("transaction failed: %v", err)
	}
}

func TestQueryInterceptor_SerializedExec(t *testing.T) {
	db := setupTestDB(t)
	qi := NewQueryInterceptor(db)

	if _, err := qi.ExecContext(context.Background(), "CREATE TABLE test (id INTEGER)"); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	done := make(chan bool, 2)

	go func() {
		for i := range 10 {
			if _, err := qi.ExecContext(context.Background(), "INSERT INTO test VALUES (?)", i); err != nil {
				t.Errorf("failed to insert: %v", err)
			}
		}
		done <- true
	}()

	go func() {
		for i := range 10 {
			if _, err := qi.ExecContext(context.Background(), "INSERT INTO test VALUES (?)", i+10); err != nil {
				t.Errorf("failed to insert: %v", err)
			}
		}
		done <- true
	}()

	<-done
	<-done

	row := qi.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM test")
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}

	if count != 20 {
		t.Errorf("expected 20 rows, got %d", count)
	}
}
