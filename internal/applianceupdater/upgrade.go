package applianceupdater

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"
)

func (c *Coordinator) applyArchive(ctx context.Context, archive string) error {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return err
	}
	id := hex.EncodeToString(random[:])
	previous, err := c.Runtime.Running(ctx)
	if err != nil {
		return c.reject(archive, "metadata", err)
	}
	// Loading into a separate tag must not change the service's image selection.
	candidate, err := c.Runtime.Load(ctx, archive, "localhost/oma-upgrade-load:"+id)
	if err != nil {
		return c.reject(archive, "load", err)
	}
	if previous.ID == candidate.ID {
		if err := c.publish(nil, Status{File: filepath.Base(archive), Result: "unchanged"}, &previous); err != nil {
			return err
		}
		return os.Remove(archive)
	}
	if err := c.checkCompatibility(ctx, previous, candidate); err != nil {
		return c.reject(archive, "compatibility", err)
	}
	if err := c.Runtime.Pin(ctx, previous, "localhost/oma-upgrade-previous:"+id); err != nil {
		return err
	}
	if err := c.Runtime.Pin(ctx, candidate, "localhost/oma-upgrade-candidate:"+id); err != nil {
		return err
	}
	attempt := &Journal{
		Version:   1,
		AttemptID: id,
		Archive:   filepath.Base(archive),
		Previous:  previous,
		Candidate: candidate,
	}
	if err := c.savePhase(attempt, PhasePreparing); err != nil {
		return err
	}
	if err := c.Runtime.Stop(ctx); err != nil {
		return c.failedRecovery(ctx, attempt, "stop", err)
	}
	if err := c.Runtime.Migrate(ctx, *attempt, "prepare"); err != nil {
		attempt.Error, attempt.ErrorStage = "migration preparation failed", "migration-prepare"
		return c.abort(ctx, attempt)
	}
	if err := c.savePhase(attempt, PhasePrepared); err != nil {
		return err
	}
	if err := c.savePhase(attempt, PhaseApplying); err != nil {
		return err
	}
	if err := c.Runtime.Migrate(ctx, *attempt, "apply"); err != nil {
		attempt.Error, attempt.ErrorStage = "forward migration failed", "migration-up"
		return c.rollback(context.WithoutCancel(ctx), attempt)
	}
	if err := c.Runtime.CheckExtensions(ctx, candidate); err != nil {
		attempt.Error, attempt.ErrorStage = "candidate extensions changed during migration", "compatibility"
		return c.rollback(context.WithoutCancel(ctx), attempt)
	}
	attempt.CommitTime = time.Now().UTC()
	// A failure after this journal write must never enter rollback, even when
	// normal startup or status publication fails.
	if err := c.savePhase(attempt, PhaseCommitted); err != nil {
		return err
	}
	return c.finishAttempt(ctx, attempt)
}

func (c *Coordinator) recoverAttempt(ctx context.Context, attempt *Journal) error {
	if !attempt.Phase.allowsStartup() && attempt.Phase != PhaseRecoveryFailed {
		if err := c.Runtime.CancelOffline(ctx, *attempt); err != nil {
			if attempt.Phase == PhasePreparing {
				// Apply was never authorized; the source may still have jobs.
				return err
			}
			return c.failedRecovery(ctx, attempt, "migration-down", err)
		}
	}
	switch attempt.Phase {
	case PhasePreparing:
		attempt.Error, attempt.ErrorStage = "interrupted before forward migration", "migration-prepare"
		return c.abort(ctx, attempt)
	case PhasePrepared, PhaseApplying, PhaseRollingBack:
		if attempt.Error == "" {
			attempt.Error, attempt.ErrorStage = "upgrade interrupted before commit", "migration-up"
		}
		return c.rollback(ctx, attempt)
	case PhaseRestoring:
		return c.restore(ctx, attempt)
	case PhaseCommitted, PhaseRolledBack:
		return c.finishAttempt(ctx, attempt)
	case PhaseAborted:
		return c.abort(ctx, attempt)
	case PhaseRecoveryFailed:
		_ = c.Runtime.Stop(ctx)
		return fmt.Errorf("attempt %s requires explicit operator recovery", attempt.AttemptID)
	default:
		return fmt.Errorf("unsupported recovery state")
	}
}

func (c *Coordinator) rollback(ctx context.Context, attempt *Journal) error {
	if err := c.savePhase(attempt, PhaseRollingBack); err != nil {
		return err
	}
	if err := c.Runtime.Stop(ctx); err != nil {
		return c.failedRecovery(ctx, attempt, "migration-down", err)
	}
	// Only the candidate contains the down SQL for its newly applied migrations.
	if err := c.Runtime.Migrate(ctx, *attempt, "rollback"); err != nil {
		return c.failedRecovery(ctx, attempt, "migration-down", err)
	}
	if err := c.Runtime.Migrate(ctx, *attempt, "verify"); err != nil {
		return c.failedRecovery(ctx, attempt, "rollback-verify", err)
	}
	attempt.RollbackVerified = true
	if err := c.savePhase(attempt, PhaseRestoring); err != nil {
		return err
	}
	return c.restore(ctx, attempt)
}

func (c *Coordinator) restore(ctx context.Context, attempt *Journal) error {
	if err := c.Runtime.CheckExtensions(ctx, attempt.Previous); err != nil {
		return c.failedRecovery(ctx, attempt, "rollback-start", err)
	}
	if err := c.savePhase(attempt, PhaseRolledBack); err != nil {
		return err
	}
	return c.finishAttempt(ctx, attempt)
}

func (c *Coordinator) abort(ctx context.Context, attempt *Journal) error {
	// Preparing never authorizes apply. No down SQL is permitted on this path.
	if err := c.savePhase(attempt, PhaseAborted); err != nil {
		return err
	}
	return c.finishAttempt(ctx, attempt)
}

func (c *Coordinator) failedRecovery(ctx context.Context, attempt *Journal, stage string, cause error) error {
	// Record a closed gate before trying to stop a process; start failures cannot
	// accidentally authorize old normal work.
	attempt.Error, attempt.ErrorStage = "recovery failed: "+stage, stage
	attempt.RecoveryFrom = attempt.Phase
	err := c.savePhase(attempt, PhaseRecoveryFailed)
	stopErr := c.Runtime.Stop(ctx)
	status := Status{File: attempt.Archive, Result: "failed", Error: attempt.Error, ErrorStage: stage}
	statusErr := c.publish(attempt, status, nil)
	return errors.Join(cause, err, stopErr, statusErr)
}

func (c *Coordinator) savePhase(attempt *Journal, phase Phase) error {
	attempt.Phase = phase
	return c.State.Save(*attempt)
}

func (c *Coordinator) checkCompatibility(ctx context.Context, previous, candidate Image) error {
	for _, image := range []Image{previous, candidate} {
		if !imageID.MatchString(image.ID) || image.Version == "" || image.Digest == "" {
			return fmt.Errorf("immutable image identities and release metadata are required")
		}
	}
	pair := Transition{Previous: previous.ID, Candidate: candidate.ID}
	if !slices.Contains(c.Config.Transitions, pair) {
		return fmt.Errorf("image pair is not in the appliance-tested transition allowlist")
	}
	for _, image := range []Image{previous, candidate} {
		if err := c.Runtime.CheckExtensions(ctx, image); err != nil {
			return err
		}
	}
	return nil
}
