package store

import (
	"context"
	"fmt"
	"hash/fnv"

	"gorm.io/gorm"
)

const (
	lockKey        = "zed_token_lock_key"
	globalLockStmt = "SELECT pg_advisory_xact_lock(%d);"
	sharedLockStmt = "SELECT pg_advisory_xact_lock_shared(%d);"
	writeStmt      = "INSERT INTO zed_token (id, token) VALUES (1, $1) ON CONFLICT (id) DO UPDATE SET token = excluded.token;"
)

// ZedTokenStore is the corrected version of ZedTokenStore
type ZedTokenStore struct {
	lockID int32
	db     *gorm.DB
}

// NewZedTokenStore creates a new ZedTokenStoreFixed (made public for testing)
func NewZedTokenStore(db *gorm.DB) *ZedTokenStore {
	h := fnv.New32a()
	h.Write([]byte(lockKey))

	return &ZedTokenStore{
		db:     db,
		lockID: int32(h.Sum32()),
	}
}

// Read reads the token (assumes caller has already acquired appropriate lock)
func (z *ZedTokenStore) Read(ctx context.Context) (*string, error) {
	var token string
	tx := z.getDB(ctx).Raw("SELECT token FROM zed_token LIMIT 1;").Scan(&token)
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to read token: %w", tx.Error)
	}

	if token == "" {
		return nil, nil
	}

	return &token, nil
}

// Write writes the token (assumes caller has already acquired appropriate lock)
func (z *ZedTokenStore) Write(ctx context.Context, token string) error {
	// upsert query to keep only one value of the token in the db
	tx := z.getDB(ctx).Exec(writeStmt, token)
	if tx.Error != nil {
		return fmt.Errorf("failed to write token: %w", tx.Error)
	}

	return nil
}

func (z *ZedTokenStore) AcquireGlobalLock(ctx context.Context) error {
	return z.acquireLock(ctx, false)
}

func (z *ZedTokenStore) AcquireSharedLock(ctx context.Context) error {
	return z.acquireLock(ctx, true)
}

// AcquireLock attempts to acquire either a shared or global advisory lock
func (z *ZedTokenStore) acquireLock(ctx context.Context, isShared bool) error {
	if FromContext(ctx) == nil {
		return fmt.Errorf("advisory xact lock requires an active transaction in context")
	}
	lockStmt := globalLockStmt
	if isShared {
		lockStmt = sharedLockStmt
	}

	tx := z.getDB(ctx).Exec(fmt.Sprintf(lockStmt, z.lockID))
	if tx.Error != nil {
		return fmt.Errorf("lock query failed: %w", tx.Error)
	}

	return nil
}

// getDB returns the database connection, preferring transaction from context
func (z *ZedTokenStore) getDB(ctx context.Context) *gorm.DB {
	tx := FromContext(ctx)
	if tx != nil {
		return tx
	}
	return z.db
}
