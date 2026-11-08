package applianceupdater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func (c *Coordinator) finishAttempt(ctx context.Context, attempt *Journal) error {
	image, err := attempt.selectedImage()
	if err != nil {
		return err
	}
	err = c.ensureRunning(ctx, image)
	if err != nil {
		c.recordStartupFailure(attempt, "normal startup failed; durable decision retained", "restart")
		return err
	}
	running, err := c.Runtime.Running(ctx)
	if err != nil || running.ID != image.ID || running.Digest == "" || running.Version != image.Version {
		c.recordStartupFailure(attempt, "running image identity or metadata mismatch", "metadata")
		return fmt.Errorf("cannot confirm running image metadata")
	}
	if attempt.Reported {
		return nil
	}
	result := "failed"
	switch attempt.Phase {
	case PhaseCommitted:
		result = "applied"
	case PhaseRolledBack:
		result = "rolled-back"
	}
	status := Status{
		File:       attempt.Archive,
		Result:     result,
		Error:      attempt.Error,
		ErrorStage: attempt.ErrorStage,
	}
	if err := c.publish(attempt, status, &running); err != nil {
		return err
	}
	// Retain journal, baseline, and image pins. Automatic pruning is deliberately
	// absent; a failed attempt must never lose its only recovery image.
	if err := os.Remove(filepath.Join(c.State.Root, attempt.Archive)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	folder, err := os.Open(c.State.Root)
	if err != nil {
		return err
	}
	if err := errors.Join(folder.Sync(), folder.Close()); err != nil {
		return err
	}
	attempt.Reported = true
	return c.State.Save(*attempt)
}

func (c *Coordinator) ensureRunning(ctx context.Context, image Image) error {
	running, err := c.Runtime.Running(ctx)
	if err == nil {
		if running.ID != image.ID {
			return fmt.Errorf("running image does not match the host decision")
		}
		return nil
	}
	// An inspection failure does not prove the process exited. Confirm that
	// the data is unused before starting the selected image.
	if err := c.Runtime.VerifyStopped(ctx); err != nil {
		return fmt.Errorf("cannot confirm healthy startup or agent exit; existing container was left running")
	}
	return c.startSelectedImage(ctx, image)
}

func (c *Coordinator) startSelectedImage(ctx context.Context, image Image) error {
	if err := c.Runtime.Stop(ctx); err != nil {
		return err
	}
	if err := c.Runtime.Select(ctx, image); err != nil {
		return err
	}
	if err := c.Runtime.Start(ctx); err != nil {
		return err
	}
	running, err := c.Runtime.Running(ctx)
	if err != nil || running.ID != image.ID || running.Version != image.Version {
		return fmt.Errorf("started container does not match the selected image")
	}
	return nil
}

// Record the retry state best-effort without hiding the original startup error.
func (c *Coordinator) recordStartupFailure(attempt *Journal, message, stage string) {
	attempt.Reported = false
	_ = c.State.Save(*attempt)
	status := Status{File: attempt.Archive, Result: "failed", Error: message, ErrorStage: stage}
	_ = c.publish(attempt, status, nil)
}

func (c *Coordinator) publish(attempt *Journal, status Status, image *Image) error {
	var previous Status
	data, err := os.ReadFile(filepath.Join(c.State.Root, "status.json"))
	if err == nil {
		// Status is a consumer-facing, extensible record, unlike host state.
		if err := json.Unmarshal(data, &previous); err != nil {
			return fmt.Errorf("previous status is unreadable: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	status.Time = time.Now().UTC()
	status.LastSuccessfulUpdate = previous.LastSuccessfulUpdate
	if image != nil {
		status.RunningImage = &RunningImage{image.Digest, image.Version}
	}
	if attempt != nil {
		status.AttemptID = attempt.AttemptID
		if attempt.RollbackVerified {
			status.MigrationRollback = &RollbackStatus{State: "verified"}
		} else if attempt.Phase == PhaseRecoveryFailed {
			status.MigrationRollback = &RollbackStatus{State: "failed", ErrorStage: attempt.ErrorStage}
		}
		if status.Result == "applied" {
			status.LastSuccessfulUpdate = &SuccessfulUpdate{*status.RunningImage, attempt.CommitTime}
		}
	}
	return c.State.write("status.json", status)
}

func (c *Coordinator) reject(archive, stage string, cause error) error {
	status := Status{File: filepath.Base(archive), Result: "failed", Error: cause.Error(), ErrorStage: stage}
	if err := c.publish(nil, status, nil); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(c.State.Root, "rejected"), 0700); err != nil {
		return err
	}
	folder, err := os.MkdirTemp(filepath.Join(c.State.Root, "rejected"), "archive-")
	if err != nil {
		return err
	}
	// Keep the archive for diagnosis without repeatedly triggering the path unit.
	return os.Rename(archive, filepath.Join(folder, filepath.Base(archive)))
}
