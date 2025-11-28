package jobs

import (
	"context"
	"encoding/base64"
	"time"

	"github.com/riverqueue/river"

	"github.com/kubev2v/migration-planner/internal/opa"
	"github.com/kubev2v/migration-planner/internal/rvtools"
)

const (
	JobTimeout = 5 * time.Minute
	JobKind    = "rvtools_parse"
)

type RVToolsArgs struct {
	OrgID    string `json:"org_id"`
	Username string `json:"username"`
	FileData string `json:"file_data"` // Base64 encoded for clean JSONB storage
}

func (RVToolsArgs) Kind() string {
	return JobKind
}

func (RVToolsArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       DefaultQueue,
		MaxAttempts: MaxJobRetries,
	}
}

type RVToolsWorker struct {
	river.WorkerDefaults[RVToolsArgs]
	opaValidator *opa.Validator
}

func NewRVToolsWorker(opaValidator *opa.Validator) *RVToolsWorker {
	return &RVToolsWorker{opaValidator: opaValidator}
}

func (w *RVToolsWorker) Timeout(job *river.Job[RVToolsArgs]) time.Duration {
	return JobTimeout
}

func (w *RVToolsWorker) Work(ctx context.Context, job *river.Job[RVToolsArgs]) error {
	// Check for cancellation before starting
	if err := ctx.Err(); err != nil {
		return err
	}

	fileData, err := base64.StdEncoding.DecodeString(job.Args.FileData)
	if err != nil {
		return err
	}

	// Check for cancellation before expensive parsing operation
	if err := ctx.Err(); err != nil {
		return err
	}

	inventory, err := rvtools.ParseRVTools(ctx, fileData, w.opaValidator)
	if err != nil {
		return err
	}

	return river.RecordOutput(ctx, inventory)
}
