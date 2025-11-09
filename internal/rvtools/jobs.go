package rvtools

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"github.com/kubev2v/migration-planner/internal/opa"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/log"
)

// RVToolsJobArgs defines the arguments for RVTools processing job
type RVToolsJobArgs struct {
	SnapshotID   uint      `json:"snapshotId"`   // The snapshot to update
	AssessmentID uuid.UUID `json:"assessmentId"` // For uniqueness constraint
	RVToolsData  string    `json:"rvtoolsData"`  // Base64 encoded file data
}

// Kind returns the unique identifier for this job type
func (RVToolsJobArgs) Kind() string {
	return "rvtools_process"
}

// RVToolsWorker processes RVTools files asynchronously
type RVToolsWorker struct {
	river.WorkerDefaults[RVToolsJobArgs]
	store        store.Store
	opaValidator *opa.Validator
}

// NewRVToolsWorker creates a new RVTools worker
func NewRVToolsWorker(store store.Store, opaValidator *opa.Validator) *RVToolsWorker {
	return &RVToolsWorker{
		store:        store,
		opaValidator: opaValidator,
	}
}

// Work processes the RVTools file and updates the snapshot
func (w *RVToolsWorker) Work(ctx context.Context, job *river.Job[RVToolsJobArgs]) error {
	logger := log.NewDebugLogger("rvtools_worker").
		WithContext(ctx).
		Operation("process_rvtools").
		WithInt("snapshot_id", int(job.Args.SnapshotID)).
		WithUUID("assessment_id", job.Args.AssessmentID).
		Build()

	logger.Step("job_started").WithInt("encoded_data_size", len(job.Args.RVToolsData)).Log()

	// Check if snapshot still exists and is in a processable state
	snapshot, err := w.store.Snapshot().Get(ctx, job.Args.SnapshotID)
	if err != nil {
		logger.Error(err).WithString("step", "get_snapshot").Log()
		return fmt.Errorf("snapshot not found or has been deleted: %w", err)
	}
	if snapshot.Status != model.SnapshotStatusPending &&
		snapshot.Status != model.SnapshotStatusParsing &&
		snapshot.Status != model.SnapshotStatusValidating {
		logger.Step("check_snapshot_status").WithString("status", string(snapshot.Status)).Log()
		return fmt.Errorf("snapshot is not in a processable state: %s", snapshot.Status)
	}
	logger.Step("snapshot_verified").WithString("status", string(snapshot.Status)).Log()

	// Decode base64 data
	content, err := base64.StdEncoding.DecodeString(job.Args.RVToolsData)
	if err != nil {
		errMsg := fmt.Sprintf("error decoding RVTools data: %v", err)
		logger.Error(err).WithString("step", "decode_base64").Log()
		if updateErr := w.store.Snapshot().Update(ctx, job.Args.SnapshotID, &model.Snapshot{
			Status: model.SnapshotStatusFailed,
			Error:  &errMsg,
		}); updateErr != nil {
			logger.Error(updateErr).WithString("step", "update_status_failed").Log()
		}
		return fmt.Errorf("failed to decode RVTools data: %w", err)
	}
	logger.Step("decoded_base64").WithInt("data_size", len(content)).Log()

	// Update snapshot status to parsing
	if err := w.store.Snapshot().Update(ctx, job.Args.SnapshotID, &model.Snapshot{
		Status: model.SnapshotStatusParsing,
	}); err != nil {
		logger.Error(err).WithString("step", "update_status_parsing").Log()
		// Continue processing even if status update fails
	}

	// Parse RVTools file
	logger.Step("parsing_rvtools").Log()
	inventory, err := ParseRVTools(ctx, content, w.opaValidator, w.store, job.Args.SnapshotID)
	if err != nil {
		errMsg := fmt.Sprintf("error parsing RVTools file: %v", err)
		logger.Error(err).WithString("step", "parse_rvtools").Log()
		if updateErr := w.store.Snapshot().Update(ctx, job.Args.SnapshotID, &model.Snapshot{
			Status: model.SnapshotStatusFailed,
			Error:  &errMsg,
		}); updateErr != nil {
			logger.Error(updateErr).WithString("step", "update_status_failed").Log()
		}
		return fmt.Errorf("failed to parse RVTools file: %w", err)
	}
	logger.Step("parsed_rvtools").Log()

	// Update snapshot with inventory and mark as ready
	if err := w.store.Snapshot().Update(ctx, job.Args.SnapshotID, &model.Snapshot{
		Inventory: inventory,
		Status:    model.SnapshotStatusReady,
	}); err != nil {
		errMsg := fmt.Sprintf("error updating snapshot: %v", err)
		logger.Error(err).WithString("step", "update_snapshot").Log()
		if updateErr := w.store.Snapshot().Update(ctx, job.Args.SnapshotID, &model.Snapshot{
			Status: model.SnapshotStatusFailed,
			Error:  &errMsg,
		}); updateErr != nil {
			logger.Error(updateErr).WithString("step", "update_status_failed").Log()
		}
		return fmt.Errorf("failed to update snapshot: %w", err)
	}

	logger.Step("snapshot_ready").Log()

	logger.Success().Log()
	return nil
}

// Timeout returns the maximum duration a job can run before being interrupted
func (w *RVToolsWorker) Timeout(job *river.Job[RVToolsJobArgs]) time.Duration {
	return 5 * time.Minute // Allow 5 minutes for RVTools processing
}
