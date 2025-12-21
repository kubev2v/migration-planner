package jobs

import (
	"context"
	"fmt"

	"github.com/kubev2v/migration-planner/pkg/opa"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/store"
)

// Client wraps the River client and provides job management functionality.
type Client struct {
	RiverClient *river.Client[pgx.Tx]
	Pool        *pgxpool.Pool
	Worker      *RVToolsWorker
}

// NewClient creates a new River client with the RVTools worker registered.
func NewClient(ctx context.Context, cfg *config.Config, s store.Store, opaValidator *opa.Validator) (*Client, error) {
	pool, err := createPgxPool(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("creating pgx pool: %w", err)
	}

	// Create worker with store
	worker := NewRVToolsWorker(opaValidator, s)

	workers := river.NewWorkers()
	river.AddWorker(workers, worker)

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 5},
		},
		Workers: workers,
	})
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("creating river client: %w", err)
	}

	return &Client{
		RiverClient: riverClient,
		Pool:        pool,
		Worker:      worker,
	}, nil
}

// Stop gracefully shuts down the job processor.
func (c *Client) Stop(ctx context.Context) error {
	if err := c.RiverClient.Stop(ctx); err != nil {
		return err
	}
	c.Pool.Close()
	return nil
}

// createPgxPool creates a pgx connection pool for River.
func createPgxPool(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error) {
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s",
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Hostname,
		cfg.Database.Port,
		cfg.Database.Name,
	)

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parsing pool config: %w", err)
	}

	poolConfig.MaxConns = 10

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("creating pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	return pool, nil
}
