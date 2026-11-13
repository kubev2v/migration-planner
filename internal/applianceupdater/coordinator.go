package applianceupdater

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Coordinator struct {
	Config  Config
	Runtime Runtime
	State   State
}

func New(c Config, runtime Runtime) (*Coordinator, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &Coordinator{Config: c, Runtime: runtime, State: State{filepath.Join(c.DataDir, "updates")}}, nil
}

func (c *Coordinator) Update(ctx context.Context) error {
	lock, err := c.State.Lock()
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	attempt, err := c.State.Active()
	if err != nil {
		return c.suspendUpdates(ctx, errors.Join(err, c.Runtime.Stop(ctx)))
	}
	if attempt != nil && !attempt.Reported {
		if err := c.recoverAttempt(ctx, attempt); err != nil {
			return c.suspendUpdates(ctx, err)
		}
	}
	archives, err := filepath.Glob(filepath.Join(c.State.Root, "*.tar"))
	if err != nil {
		return err
	}
	for _, archive := range archives {
		info, err := os.Lstat(archive)
		if err != nil || !info.Mode().IsRegular() {
			return c.suspendUpdates(ctx, fmt.Errorf("published archive must be a regular file"))
		}
		if err := c.applyArchive(ctx, archive); err != nil {
			return c.suspendUpdates(ctx, err)
		}
	}
	return nil
}

// Recover reconciles unfinished offline work before normal application startup.
func (c *Coordinator) Recover(ctx context.Context) error {
	lock, err := c.State.Lock()
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	attempt, err := c.State.Active()
	if err != nil {
		_ = c.Runtime.Stop(ctx)
		return c.suspendUpdates(ctx, err)
	}
	if attempt == nil {
		return nil
	}
	if err := c.recoverAttempt(ctx, attempt); err != nil {
		return c.suspendUpdates(ctx, err)
	}
	return nil
}

// RetryRecovery is an explicit operator action after correcting a recovery
// fault. It cannot reopen a committed attempt or target a different attempt.
func (c *Coordinator) RetryRecovery(ctx context.Context, id string) error {
	lock, err := c.State.Lock()
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	attempt, err := c.State.Active()
	if err != nil {
		return err
	}
	if attempt == nil || attempt.Phase != PhaseRecoveryFailed || attempt.AttemptID != id || !attempt.CommitTime.IsZero() {
		return fmt.Errorf("retry requires the exact unresolved pre-commit recovery attempt")
	}
	if err := c.Runtime.CancelOffline(ctx, *attempt); err != nil {
		return c.suspendUpdates(ctx, err)
	}
	if attempt.RecoveryFrom == PhasePreparing {
		err = c.abort(ctx, attempt)
	} else {
		err = c.rollback(ctx, attempt)
	}
	if err != nil {
		return c.suspendUpdates(ctx, err)
	}
	return nil
}

// Gate runs before every service start, outside the coordinator lock. This is
// intentional: the coordinator waits for systemctl start, which waits for Gate.
func (c *Coordinator) Gate(ctx context.Context) error {
	attempt, err := c.State.Active()
	if err != nil {
		return err
	}
	env := ""
	if attempt != nil {
		image, err := attempt.selectedImage()
		if err != nil {
			return err
		}
		selected, err := c.Runtime.Inspect(ctx, c.Config.ImageTag)
		if err != nil || selected.ID != image.ID {
			return fmt.Errorf("selected service image does not match the host decision")
		}
		env += "AGENT_AUTO_MIGRATE=false\n"
	}
	return AtomicWrite(c.Config.EnvironmentFile, []byte(env), 0600)
}

func (c *Coordinator) suspendUpdates(ctx context.Context, err error) error {
	return errors.Join(err, c.Runtime.Suspend(context.WithoutCancel(ctx)))
}
