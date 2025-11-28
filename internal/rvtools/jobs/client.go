package jobs

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/kubev2v/migration-planner/internal/opa"
)

const (
	DefaultQueue  = "rvtools"
	MaxJobRetries = 1
)

type Client struct {
	*river.Client[pgx.Tx]
}

func NewClient(ctx context.Context, pool *pgxpool.Pool, opaValidator *opa.Validator) (*Client, error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, NewRVToolsWorker(opaValidator))

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			DefaultQueue: {MaxWorkers: 10},
		},
		Workers: workers,
	})
	if err != nil {
		return nil, err
	}

	return &Client{Client: riverClient}, nil
}

func (c *Client) InsertJob(ctx context.Context, args RVToolsArgs) (int64, error) {
	result, err := c.Insert(ctx, args, &river.InsertOpts{
		Queue:       DefaultQueue,
		MaxAttempts: MaxJobRetries,
	})
	if err != nil {
		return 0, err
	}
	return result.Job.ID, nil
}
