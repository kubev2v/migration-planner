// Package applianceupdater coordinates host-side agent upgrades. Database migration
// semantics belong to the pinned agent CLI, never to the host.
package applianceupdater

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"time"
)

var imageID = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var attemptID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

type Image struct {
	ID      string `json:"id"`
	Digest  string `json:"digest"`
	Version string `json:"version"`
}

// Transition pins the exact images accepted together in appliance testing.
// An empty policy intentionally disables activation, including schema changes.
type Transition struct {
	Previous  string `json:"previous_image"`
	Candidate string `json:"candidate_image"`
}

type Config struct {
	DataDir         string       `json:"data_dir"`
	ImageTag        string       `json:"image_tag"`
	EnvironmentFile string       `json:"environment_file"`
	CommandSeconds  int          `json:"command_seconds"`
	Transitions     []Transition `json:"transitions"`
}

func (c Config) Validate() error {
	for _, path := range []string{c.DataDir, c.EnvironmentFile} {
		if !filepath.IsAbs(path) {
			return fmt.Errorf("host paths must be absolute")
		}
	}
	if c.ImageTag == "" || c.CommandSeconds < 1 {
		return fmt.Errorf("image tag and positive deadlines are required")
	}
	for _, pair := range c.Transitions {
		if !imageID.MatchString(pair.Previous) || !imageID.MatchString(pair.Candidate) || pair.Previous == pair.Candidate {
			return fmt.Errorf("invalid tested transition")
		}
	}
	return nil
}

// Phase records which operations an upgrade attempt may perform after a restart.
// The string values are persisted in the host journal.
type Phase string

const (
	PhasePreparing      Phase = "preparing"
	PhasePrepared       Phase = "prepared"
	PhaseApplying       Phase = "applying"
	PhaseRollingBack    Phase = "rolling-back"
	PhaseRestoring      Phase = "restoring"
	PhaseCommitted      Phase = "committed"
	PhaseRolledBack     Phase = "rolled-back"
	PhaseAborted        Phase = "aborted"
	PhaseRecoveryFailed Phase = "recovery-failed"
)

func (p Phase) allowsStartup() bool {
	switch p {
	case PhaseCommitted, PhaseRolledBack, PhaseAborted:
		return true
	default:
		return false
	}
}

type Journal struct {
	Version          int       `json:"version"`
	AttemptID        string    `json:"attempt_id"`
	Archive          string    `json:"archive"`
	Previous         Image     `json:"previous"`
	Candidate        Image     `json:"candidate"`
	Phase            Phase     `json:"phase"`
	CommitTime       time.Time `json:"commit_time,omitempty"`
	Error            string    `json:"error,omitempty"`
	ErrorStage       string    `json:"error_stage,omitempty"`
	Reported         bool      `json:"reported"`
	RollbackVerified bool      `json:"rollback_verified"`
	RecoveryFrom     Phase     `json:"recovery_from,omitempty"`
}

// selectedImage allows normal startup only after the host has resolved the attempt.
func (attempt Journal) selectedImage() (Image, error) {
	switch attempt.Phase {
	case PhaseCommitted:
		return attempt.Candidate, nil
	case PhaseRolledBack, PhaseAborted:
		return attempt.Previous, nil
	default:
		return Image{}, fmt.Errorf("phase %s blocks application startup", attempt.Phase)
	}
}

type RunningImage struct {
	Digest  string `json:"digest"`
	Version string `json:"version"`
}

type SuccessfulUpdate struct {
	RunningImage
	Time time.Time `json:"time"`
}

type Status struct {
	File                 string            `json:"file"`
	Result               string            `json:"result"`
	Time                 time.Time         `json:"time"`
	Error                string            `json:"error"`
	ErrorStage           string            `json:"error_stage"`
	AttemptID            string            `json:"attempt_id,omitempty"`
	MigrationRollback    *RollbackStatus   `json:"migration_rollback,omitempty"`
	RunningImage         *RunningImage     `json:"running_image"`
	LastSuccessfulUpdate *SuccessfulUpdate `json:"last_successful_update"`
}

type RollbackStatus struct {
	State      string `json:"state"`
	ErrorStage string `json:"error_stage,omitempty"`
}

// Runtime isolates host commands from the durable state machine. Tests
// substitute host operations, but use real journal files.
type Runtime interface {
	Running(context.Context) (Image, error)
	Inspect(context.Context, string) (Image, error)
	Load(context.Context, string, string) (Image, error)
	Pin(context.Context, Image, string) error
	CheckExtensions(context.Context, Image) error
	Stop(context.Context) error
	VerifyStopped(context.Context) error
	Start(context.Context) error
	Select(context.Context, Image) error
	Migrate(context.Context, Journal, string) error
	CancelOffline(context.Context, Journal) error
	Suspend(context.Context) error
}
