package store

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

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

// setupTestDBWithMaxConns creates a file-backed DuckDB (WAL-enabled) with MaxOpenConns(1),
// matching production configuration.
func setupTestDBWithMaxConns(t *testing.T, maxConns int) *sql.DB {
	t.Helper()
	dbPath := t.TempDir() + "/test.db"
	db, err := sql.Open("duckdb", dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	db.SetMaxOpenConns(maxConns)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close database: %v", err)
		}
	})
	return db
}

// TestQueryInterceptor_ConcurrentExecWithSingleConn verifies that concurrent
// non-transactional writes complete without hanging when MaxOpenConns is 1.
// This is the scenario that caused ECOPROJECT-5221: with FORCE CHECKPOINT after
// every ExecContext, concurrent writes would deadlock because the single connection
// was held by the checkpoint while the next write waited for the same connection.
func TestQueryInterceptor_ConcurrentExecWithSingleConn(t *testing.T) {
	db := setupTestDBWithMaxConns(t, 1)
	qi := NewQueryInterceptor(db)

	if _, err := qi.ExecContext(context.Background(), "CREATE TABLE test (id INTEGER, data VARCHAR)"); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	const (
		numGoroutines = 10
		numWrites     = 20
		timeout       = 30 * time.Second
	)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var wg sync.WaitGroup
	errCh := make(chan error, numGoroutines*numWrites)

	for g := range numGoroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range numWrites {
				if _, err := qi.ExecContext(ctx, "INSERT INTO test VALUES (?, ?)", g*numWrites+i, "data"); err != nil {
					errCh <- err
					return
				}
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("concurrent writes with MaxOpenConns(1) timed out after %v — possible deadlock", timeout)
	}

	close(errCh)
	for err := range errCh {
		t.Errorf("write error: %v", err)
	}

	row := qi.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM test")
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}
	if count != numGoroutines*numWrites {
		t.Errorf("expected %d rows, got %d", numGoroutines*numWrites, count)
	}
}

// TestQueryInterceptor_ConcurrentReadsAndWritesSingleConn verifies that mixed
// concurrent reads and writes don't deadlock with a single connection. This
// simulates the production scenario where the console background loop (reads +
// deletes) runs alongside API write handlers.
func TestQueryInterceptor_ConcurrentReadsAndWritesSingleConn(t *testing.T) {
	db := setupTestDBWithMaxConns(t, 1)
	qi := NewQueryInterceptor(db)

	if _, err := qi.ExecContext(context.Background(), "CREATE TABLE test (id INTEGER, data VARCHAR)"); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	const (
		numWriters = 5
		numReaders = 5
		numOps     = 20
		timeout    = 30 * time.Second
	)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var wg sync.WaitGroup

	for w := range numWriters {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range numOps {
				if _, err := qi.ExecContext(ctx, "INSERT INTO test VALUES (?, ?)", w*numOps+i, "data"); err != nil {
					t.Errorf("write error: %v", err)
					return
				}
			}
		}()
	}

	for range numReaders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range numOps {
				row := qi.QueryRowContext(ctx, "SELECT COUNT(*) FROM test")
				var count int
				if err := row.Scan(&count); err != nil {
					t.Errorf("read error: %v", err)
					return
				}
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("concurrent reads/writes with MaxOpenConns(1) timed out after %v — possible deadlock", timeout)
	}
}

// TestQueryInterceptor_ConcurrentTxAndNonTxWritesSingleConn verifies that
// transactional and non-transactional writes interleaved across goroutines
// complete without hanging. This simulates API handlers (transactional) running
// alongside background outbox cleanup (non-transactional deletes).
func TestQueryInterceptor_ConcurrentTxAndNonTxWritesSingleConn(t *testing.T) {
	db := setupTestDBWithMaxConns(t, 1)
	qi := NewQueryInterceptor(db)
	transactor := NewTransactor(db)

	if _, err := qi.ExecContext(context.Background(), "CREATE TABLE test (id INTEGER, data VARCHAR)"); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	const (
		numTxWriters    = 3
		numNonTxWriters = 3
		numOps          = 15
		timeout         = 30 * time.Second
	)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var wg sync.WaitGroup

	for w := range numTxWriters {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range numOps {
				err := transactor.WithTx(ctx, func(txCtx context.Context) error {
					_, err := qi.ExecContext(txCtx, "INSERT INTO test VALUES (?, ?)", w*1000+i, "tx")
					return err
				})
				if err != nil {
					t.Errorf("tx write error: %v", err)
					return
				}
			}
		}()
	}

	for w := range numNonTxWriters {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range numOps {
				if _, err := qi.ExecContext(ctx, "INSERT INTO test VALUES (?, ?)", (w+numTxWriters)*1000+i, "non-tx"); err != nil {
					t.Errorf("non-tx write error: %v", err)
					return
				}
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("concurrent tx + non-tx writes with MaxOpenConns(1) timed out after %v — possible deadlock", timeout)
	}

	row := qi.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM test")
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}
	expected := (numTxWriters + numNonTxWriters) * numOps
	if count != expected {
		t.Errorf("expected %d rows, got %d", expected, count)
	}
}
