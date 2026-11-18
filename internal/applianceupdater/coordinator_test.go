package applianceupdater

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

var oldImage = Image{ID: "sha256:" + strings.Repeat("a", 64), Digest: "sha256:old-digest", Version: "v0.31.0"}
var newImage = Image{ID: "sha256:" + strings.Repeat("b", 64), Digest: "sha256:new-digest", Version: "v0.32.0", MinUpgradeVersion: "v0.31.0"}

type testHost struct {
	c                   *Coordinator
	log                 []string
	selected, running   Image
	stopped             bool
	fail                string
	publicationFailure  bool
	wrongCandidateImage bool
	extensionChecks     int
	failInspection      bool
	startingChecks      int
	inspectError        error
	stopError           error
	cancelPrepare       context.CancelFunc
	cancelOfflineError  error
	verifyStoppedError  error
}

func (h *testHost) Running(ctx context.Context) (Image, error) {
	if err := ctx.Err(); err != nil {
		return Image{}, err
	}
	if h.stopped || h.failInspection {
		return Image{}, errors.New("container unavailable")
	}
	if h.startingChecks != 0 {
		if h.startingChecks > 0 {
			h.startingChecks--
		}
		return h.running, errAgentStarting
	}
	return h.running, nil
}
func (h *testHost) Inspect(_ context.Context, _ string) (Image, error) {
	return h.selected, h.inspectError
}
func (h *testHost) Load(_ context.Context, _, tag string) (Image, error) {
	h.log = append(h.log, "load "+tag)
	return newImage, nil
}
func (h *testHost) Pin(_ context.Context, _ Image, _ string) error { return nil }
func (h *testHost) CheckExtensions(_ context.Context, image Image) error {
	h.log = append(h.log, "extensions "+image.Version)
	h.extensionChecks++
	if h.fail == "extensions" || (h.fail == "extensions-recheck" && h.extensionChecks == 3) {
		return errors.New("extensions are incompatible")
	}
	return nil
}
func (h *testHost) VerifyStopped(ctx context.Context) error {
	h.log = append(h.log, "verify-stopped")
	if err := ctx.Err(); err != nil {
		return err
	}
	if h.verifyStoppedError != nil {
		return h.verifyStoppedError
	}
	if !h.stopped {
		return errors.New("container is still running")
	}
	return nil
}
func (h *testHost) Stop(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	h.log = append(h.log, "stop")
	if j, err := h.c.State.Active(); err == nil && j != nil {
		h.log = append(h.log, "stop "+string(j.Phase))
	}
	if h.fail == "stop" {
		return errors.New("container still running")
	}
	if h.stopError != nil {
		return h.stopError
	}
	h.stopped = true
	return nil
}
func (h *testHost) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := h.c.Gate(ctx); err != nil {
		return err
	}
	if !h.stopped && h.startingChecks != 0 {
		h.log = append(h.log, "wait-for-startup")
		if h.startingChecks < 0 {
			<-ctx.Done()
			return ctx.Err()
		}
		h.startingChecks = 0
		return nil
	}
	j, err := h.c.State.Active()
	if err != nil {
		return err
	}
	h.log = append(h.log, "start "+h.selected.Version)
	h.log = append(h.log, "start "+h.selected.Version+" normal")
	if h.fail == "candidate-start" && h.selected == newImage || h.fail == "old-start" && h.selected == oldImage {
		return errors.New("startup failed")
	}
	h.running, h.stopped = h.selected, false
	if h.wrongCandidateImage && h.selected == newImage {
		h.running = oldImage
	}
	if j.Phase == "committed" && h.publicationFailure {
		_ = os.Remove(filepath.Join(h.c.State.Root, "status.json"))
		_ = os.Mkdir(filepath.Join(h.c.State.Root, "status.json"), 0700)
	}
	return nil
}
func (h *testHost) Select(_ context.Context, image Image) error {
	h.log = append(h.log, "select "+image.Version)
	h.selected = image
	return nil
}
func (h *testHost) Migrate(ctx context.Context, j Journal, operation string) error {
	h.log = append(h.log, operation)
	if !h.stopped || j.Candidate != newImage {
		return errors.New("offline CLI used wrong image or shared a live application")
	}
	durable, err := h.c.State.Active()
	if err != nil {
		return err
	}
	wanted := map[string]Phase{"prepare": "preparing", "apply": "applying", "rollback": "rolling-back", "verify": "rolling-back"}
	if durable.Phase != wanted[operation] {
		return errors.New("operation preceded its durable intent")
	}
	if operation == "prepare" && h.cancelPrepare != nil {
		h.cancelPrepare()
		return ctx.Err()
	}
	if h.fail == operation {
		return errors.New("injected CLI failure")
	}
	return nil
}
func (h *testHost) Suspend(context.Context) error {
	h.log = append(h.log, "suspend-watcher")
	return nil
}

func (h *testHost) CancelOffline(ctx context.Context, _ Journal) error {
	h.log = append(h.log, "cancel-offline-cli")
	if err := ctx.Err(); err != nil {
		return err
	}
	if h.cancelOfflineError != nil {
		return h.cancelOfflineError
	}
	if h.fail == "cancel-offline" {
		return errors.New("offline container cleanup failed")
	}
	return nil
}

func fixture(t *testing.T) (*Coordinator, *testHost) {
	t.Helper()
	folder := t.TempDir()
	cfg := Config{DataDir: folder, ImageTag: "localhost/appliance:latest", EnvironmentFile: filepath.Join(folder, "upgrade.env"), CommandSeconds: 5}
	h := &testHost{selected: oldImage, running: oldImage}
	c, err := New(cfg, h)
	if err != nil {
		t.Fatal(err)
	}
	h.c = c
	if err := os.MkdirAll(filepath.Join(c.State.Root, ".staging"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(c.State.Root, "new.tar"), []byte("archive"), 0600); err != nil {
		t.Fatal(err)
	}
	return c, h
}

func readStatus(t *testing.T, c *Coordinator) Status {
	t.Helper()
	var status Status
	if err := c.State.read("status.json", &status); err != nil {
		t.Fatal(err)
	}
	return status
}

func requireOrder(t *testing.T, log []string, events ...string) {
	t.Helper()
	index := -1
	for _, event := range events {
		next := slices.Index(log[index+1:], event)
		if next < 0 {
			t.Fatalf("missing/out-of-order %s in %v", event, log)
		}
		index += next + 1
	}
}

func TestUpgradeMigratesOfflineBeforeNormalStartup(t *testing.T) {
	c, h := fixture(t)
	if err := c.Update(context.Background()); err != nil {
		t.Fatal(err)
	}
	requireOrder(t, h.log, "stop", "prepare", "apply", "select v0.32.0", "start v0.32.0 normal")
	status := readStatus(t, c)
	j, err := c.State.Active()
	if err != nil || j.Phase != "committed" || !j.Reported || status.Result != "applied" || status.LastSuccessfulUpdate == nil || !status.LastSuccessfulUpdate.Time.Equal(j.CommitTime) {
		t.Fatalf("invalid final state: %+v %+v %v", j, status, err)
	}
	if _, err := os.Stat(filepath.Join(c.State.Root, "new.tar")); !os.IsNotExist(err) {
		t.Fatal("resolved archive retained")
	}
	before, _ := os.ReadFile(filepath.Join(c.State.Root, "status.json"))
	if err := c.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(c.State.Root, "status.json"))
	if string(before) != string(after) {
		t.Fatal("boot rewrote completed status")
	}
}

func TestCandidateFailureRollsBackBeforeOldStart(t *testing.T) {
	for _, failure := range []string{"apply"} {
		t.Run(failure, func(t *testing.T) {
			c, h := fixture(t)
			h.fail = failure
			if err := c.Update(context.Background()); err != nil {
				t.Fatal(err)
			}
			requireOrder(t, h.log, "apply", "rollback", "verify", "select v0.31.0", "start v0.31.0 normal")
			if status := readStatus(t, c); status.Result != "rolled-back" || status.LastSuccessfulUpdate != nil {
				t.Fatalf("invalid rollback outcome: %+v", status)
			}
		})
	}
}

func TestFailedRecoveryNeverStartsOldImage(t *testing.T) {
	for _, failure := range []string{"rollback", "verify"} {
		t.Run(failure, func(t *testing.T) {
			c, h := fixture(t)
			lock, err := c.State.Lock()
			if err != nil {
				t.Fatal(err)
			}
			j := Journal{Version: 1, AttemptID: "interrupted", Archive: "new.tar", Previous: oldImage, Candidate: newImage}
			if err := c.savePhase(&j, "applying"); err != nil {
				t.Fatal(err)
			}
			_ = lock.Close()
			h.fail = failure
			if err := c.Recover(context.Background()); err == nil {
				t.Fatal("recovery failure accepted")
			}
			active, err := c.State.Active()
			if err != nil || active.Phase != "recovery-failed" || !h.stopped || !slices.Contains(h.log, "suspend-watcher") {
				t.Fatalf("recovery did not close gate: %+v %v", active, h.log)
			}
			if slices.Contains(h.log, "start v0.31.0") {
				t.Fatalf("old image started before successful recovery: %v", h.log)
			}
			if err := c.Gate(context.Background()); err == nil {
				t.Fatal("recovery failure allowed a manual/restart start")
			}
			if _, err := os.Stat(filepath.Join(c.State.Root, "new.tar")); err != nil {
				t.Fatal("unresolved archive lost")
			}
		})
	}
}

func TestStatusFailureAfterCommitNeverRunsDownSQL(t *testing.T) {
	c, h := fixture(t)
	h.publicationFailure = true
	if err := c.Update(context.Background()); err == nil {
		t.Fatal("status failure accepted")
	}
	j, err := c.State.Active()
	if err != nil || j.Phase != "committed" || j.Reported || slices.Contains(h.log, "rollback") {
		t.Fatalf("commit lost after publication failure: %+v %v", j, h.log)
	}
	commitTime := j.CommitTime
	if err := os.Remove(filepath.Join(c.State.Root, "status.json")); err != nil {
		t.Fatal(err)
	}
	h.publicationFailure = false
	h.log = nil
	if err := c.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if status := readStatus(t, c); status.Result != "applied" || !status.LastSuccessfulUpdate.Time.Equal(commitTime) || len(h.log) != 0 {
		t.Fatalf("report retry restarted or rolled back committed application: %+v %v", status, h.log)
	}
}

func TestUnsupportedImagesRejectBeforeDowntime(t *testing.T) {
	for _, unsupported := range []string{"minimum-version", "channel", "invalid-version", "extensions"} {
		t.Run(unsupported, func(t *testing.T) {
			c, h := fixture(t)
			switch unsupported {
			case "minimum-version":
				h.running.Version = "v0.30.0"
			case "channel":
				h.running.Version = "main-release-0.31-abcdef0"
			case "invalid-version":
				h.running.Version = "unknown"
			default:
				h.fail = unsupported
			}
			if err := c.Update(context.Background()); err != nil {
				t.Fatal(err)
			}
			if slices.Contains(h.log, "stop") || readStatus(t, c).ErrorStage != "compatibility" {
				t.Fatalf("unsupported image pair changed the source: %v", h.log)
			}
			matches, err := filepath.Glob(filepath.Join(c.State.Root, "rejected/archive-*/new.tar"))
			if err != nil || len(matches) != 1 {
				t.Fatal("rejected archive lost or still watched")
			}
		})
	}
}

func TestGateAndRecoveryAcrossRebootPhases(t *testing.T) {
	for _, phase := range []Phase{"preparing", "prepared", "applying", "rolling-back", "restoring", "committed", "rolled-back", "aborted", "recovery-failed"} {
		t.Run(string(phase), func(t *testing.T) {
			c, h := fixture(t)
			lock, err := c.State.Lock()
			if err != nil {
				t.Fatal(err)
			}
			j := Journal{Version: 1, AttemptID: "reboot", Archive: "new.tar", Previous: oldImage, Candidate: newImage}
			if phase == PhaseCommitted {
				j.CommitTime = time.Now().UTC()
			}
			j.RollbackVerified = phase == "restoring" || phase == "rolled-back"
			if phase == "recovery-failed" {
				j.RecoveryFrom = "rolling-back"
			}
			if err := c.savePhase(&j, phase); err != nil {
				t.Fatal(err)
			}
			_ = lock.Close()
			h.stopped = true
			if phase == "committed" {
				h.selected = newImage
			}
			if phase == "restoring" || phase == "applying" || phase == "preparing" || phase == "prepared" || phase == "rolling-back" || phase == "recovery-failed" {
				if err := c.Gate(context.Background()); err == nil {
					t.Fatal("unfinished attempt allowed normal startup")
				}
			}
			err = c.Recover(context.Background())
			if phase == "recovery-failed" {
				if err == nil || !h.stopped {
					t.Fatal("unresolved failure allowed startup")
				}
				err = c.RetryRecovery(context.Background(), j.AttemptID)
			}
			if err != nil {
				t.Fatal(err)
			}
			if phase == "prepared" || phase == "applying" || phase == "rolling-back" || phase == "recovery-failed" {
				requireOrder(t, h.log, "rollback", "verify", "start v0.31.0")
			} else if slices.Contains(h.log, "rollback") {
				t.Fatalf("unexpected down SQL in %s: %v", phase, h.log)
			}
		})
	}
}

func TestJournalAndCoordinatorExclusion(t *testing.T) {
	c, _ := fixture(t)
	lock, err := c.State.Lock()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	if _, err := c.State.Lock(); err == nil {
		t.Fatal("concurrent host coordinator allowed")
	}
	if err := c.State.write("host/active.json", map[string]string{"attempt_id": "../escape"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.State.Active(); err == nil {
		t.Fatal("malformed active pointer accepted")
	}
}

func TestGateRejectsMissingJournalAndMismatchedSelection(t *testing.T) {
	c, h := fixture(t)
	lock, err := c.State.Lock()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	j := Journal{Version: 1, AttemptID: "gate", Archive: "new.tar", Previous: oldImage, Candidate: newImage, CommitTime: time.Now().UTC()}
	if err := c.savePhase(&j, "committed"); err != nil {
		t.Fatal(err)
	}
	if err := c.Gate(context.Background()); err == nil || !strings.Contains(err.Error(), oldImage.ID) || !strings.Contains(err.Error(), newImage.ID) {
		t.Fatalf("gate did not explain the selected image mismatch: %v", err)
	}
	h.selected = newImage
	if err := c.Gate(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(c.Config.EnvironmentFile)
	if !strings.Contains(string(data), "AGENT_AUTO_MIGRATE=false") || strings.Contains(string(data), "AGENT_MAINTENANCE_TIMEOUT") || strings.Contains(string(data), "AGENT_UPGRADE_") {
		t.Fatalf("normal start has incorrect migration settings: %s", data)
	}
	if err := os.Remove(filepath.Join(c.State.Root, "host/active.json")); err != nil {
		t.Fatal(err)
	}
	if err := c.Gate(context.Background()); err == nil {
		t.Fatal("orphan journal allowed unmanaged startup")
	}
}

func TestAbsentStagedFilesNeverActivate(t *testing.T) {
	c, h := fixture(t)
	if err := os.Rename(filepath.Join(c.State.Root, "new.tar"), filepath.Join(c.State.Root, ".staging/new.tar")); err != nil {
		t.Fatal(err)
	}
	if err := c.Update(context.Background()); err != nil || len(h.log) != 0 {
		t.Fatalf("staging activated: %v %v", err, h.log)
	}
	if err := os.Symlink(filepath.Join(c.State.Root, ".staging/new.tar"), filepath.Join(c.State.Root, "link.tar")); err != nil {
		t.Fatal(err)
	}
	if err := c.Update(context.Background()); err == nil {
		t.Fatal("symlink handoff accepted")
	}
}

func TestStatusRetainsLastSuccess(t *testing.T) {
	c, _ := fixture(t)
	previous := Status{Time: time.Now().UTC(), Result: "applied", LastSuccessfulUpdate: &SuccessfulUpdate{RunningImage{oldImage.Digest, oldImage.Version}, time.Now().UTC()}}
	data, _ := json.Marshal(previous)
	if err := os.WriteFile(filepath.Join(c.State.Root, "status.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := c.publish(nil, Status{File: "failure.tar", Result: "failed", Error: "preflight failed", ErrorStage: "compatibility"}, nil); err != nil {
		t.Fatal(err)
	}
	if readStatus(t, c).LastSuccessfulUpdate.Version != oldImage.Version {
		t.Fatal("failure discarded last successful image")
	}
}

func TestCommittedStartupFailureRetriesCandidateOnly(t *testing.T) {
	c, h := fixture(t)
	if err := c.Update(context.Background()); err != nil {
		t.Fatal(err)
	}
	j, err := c.State.Active()
	if err != nil {
		t.Fatal(err)
	}
	commitTime := j.CommitTime
	// Recovery retries an unfinished startup/report, not later service failures.
	j.Reported = false
	if err := c.State.Save(*j); err != nil {
		t.Fatal(err)
	}
	h.stopped, h.fail = true, "candidate-start"
	if err := c.Recover(context.Background()); err == nil {
		t.Fatal("failed committed startup accepted")
	}
	j, err = c.State.Active()
	if err != nil || j.Phase != "committed" || j.Reported || readStatus(t, c).Result != "failed" {
		t.Fatalf("post-commit startup failure lost retry state: %+v %v", j, err)
	}
	if err := c.RetryRecovery(context.Background(), j.AttemptID); err == nil {
		t.Fatal("operator retry authorized down SQL after commit")
	}
	h.fail, h.log = "", nil
	if err := c.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if status := readStatus(t, c); status.Result != "applied" || !status.LastSuccessfulUpdate.Time.Equal(commitTime) || slices.Contains(h.log, "rollback") {
		t.Fatalf("candidate retry lost commit or invoked rollback: %+v %v", status, h.log)
	}
}

func TestExplicitRecoveryRetryRequiresExactFailedAttempt(t *testing.T) {
	c, h := fixture(t)
	lock, err := c.State.Lock()
	if err != nil {
		t.Fatal(err)
	}
	j := Journal{Version: 1, AttemptID: "retry", Archive: "new.tar", Previous: oldImage, Candidate: newImage}
	if err := c.savePhase(&j, "applying"); err != nil {
		t.Fatal(err)
	}
	_ = lock.Close()
	h.fail = "verify"
	if err := c.Recover(context.Background()); err == nil {
		t.Fatal("verification failure accepted")
	}
	if err := c.RetryRecovery(context.Background(), "wrong-attempt"); err == nil {
		t.Fatal("wrong attempt resumed recovery")
	}
	h.fail, h.log = "", nil
	if err := c.RetryRecovery(context.Background(), "retry"); err != nil {
		t.Fatal(err)
	}
	requireOrder(t, h.log, "rollback", "verify", "start v0.31.0 normal")
}

func TestWrongNormalContainerDoesNotReopenRollback(t *testing.T) {
	c, h := fixture(t)
	h.wrongCandidateImage = true
	if err := c.Update(context.Background()); err == nil {
		t.Fatal("wrong running container accepted")
	}
	j, err := c.State.Active()
	if err != nil || j.Phase != "committed" || slices.Contains(h.log, "rollback") {
		t.Fatalf("normal startup failure reopened rollback: %+v %v", j, h.log)
	}
}

func TestMalformedJournalStopsApplicationAndWatcher(t *testing.T) {
	c, h := fixture(t)
	lock, err := c.State.Lock()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.State.write("host/active.json", map[string]string{"attempt_id": "missing-journal"}); err != nil {
		t.Fatal(err)
	}
	_ = lock.Close()
	if err := c.Update(context.Background()); err == nil || !h.stopped || !slices.Contains(h.log, "suspend-watcher") || slices.Contains(h.log, "apply") {
		t.Fatalf("malformed state did not close the application and watcher: %v %v", err, h.log)
	}
}

func TestStopFailurePreventsOfflineWork(t *testing.T) {
	c, h := fixture(t)
	h.fail = "stop"
	if err := c.Update(context.Background()); err == nil {
		t.Fatal("stop failure accepted")
	}
	if slices.Contains(h.log, "prepare") || slices.Contains(h.log, "apply") || slices.Contains(h.log, "rollback") || slices.Contains(h.log, "verify") {
		t.Fatalf("offline migration ran with a live process: %v", h.log)
	}
}

func TestExtensionChangeBeforeCommitRollsBack(t *testing.T) {
	c, h := fixture(t)
	h.fail = "extensions-recheck"
	if err := c.Update(context.Background()); err != nil {
		t.Fatal(err)
	}
	if readStatus(t, c).Result != "rolled-back" {
		t.Fatalf("unvalidated extensions committed: %v", h.log)
	}
}

func TestNormalStartupFailureAfterCommitNeverRollsBack(t *testing.T) {
	c, h := fixture(t)
	h.fail = "candidate-start"
	if err := c.Update(context.Background()); err == nil {
		t.Fatal("normal startup failure accepted")
	}
	j, err := c.State.Active()
	if err != nil || j.Phase != "committed" || slices.Contains(h.log, "rollback") {
		t.Fatalf("post-commit failure rolled back: %+v %v", j, h.log)
	}
	h.fail, h.log = "", nil
	if err := c.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(h.log, "rollback") || readStatus(t, c).Result != "applied" {
		t.Fatalf("commit retry failed: %v", h.log)
	}
}

func TestCleanupFailureBeforeApplyDoesNotStopSourceJobs(t *testing.T) {
	c, h := fixture(t)
	lock, err := c.State.Lock()
	if err != nil {
		t.Fatal(err)
	}
	j := Journal{Version: 1, AttemptID: "prepare-cleanup-failure", Archive: "new.tar", Previous: oldImage, Candidate: newImage}
	if err := c.savePhase(&j, "preparing"); err != nil {
		t.Fatal(err)
	}
	_ = lock.Close()
	h.fail = "cancel-offline"
	if err := c.Recover(context.Background()); err == nil {
		t.Fatal("failed cleanup accepted")
	}
	if h.stopped || slices.Contains(h.log, "stop") || slices.Contains(h.log, "rollback") {
		t.Fatalf("cleanup failure before apply interrupted jobs: %v", h.log)
	}
}

func TestPreparationFailureRestartsPreviousImageWithoutRollback(t *testing.T) {
	c, h := fixture(t)
	h.fail = "prepare"
	if err := c.Update(context.Background()); err != nil {
		t.Fatal(err)
	}
	requireOrder(t, h.log, "stop", "prepare", "cancel-offline-cli", "verify-stopped", "select v0.31.0", "start v0.31.0 normal")
	if slices.Contains(h.log, "apply") || slices.Contains(h.log, "rollback") || readStatus(t, c).ErrorStage != "migration-prepare" {
		t.Fatalf("preparation failure ran migrations: %v", h.log)
	}
}

func TestUncertainContainerStateDoesNotRestartCommittedAgent(t *testing.T) {
	c, h := fixture(t)
	if err := c.Update(context.Background()); err != nil {
		t.Fatal(err)
	}
	j, err := c.State.Active()
	if err != nil {
		t.Fatal(err)
	}
	j.Reported = false
	if err := c.State.Save(*j); err != nil {
		t.Fatal(err)
	}
	h.failInspection, h.log = true, nil
	if err := c.Recover(context.Background()); err == nil {
		t.Fatal("unconfirmed running image accepted")
	}
	if slices.Contains(h.log, "stop") || slices.Contains(h.log, "rollback") {
		t.Fatalf("inspection failure restarted or rolled back the agent: %v", h.log)
	}
	h.failInspection = false
	if err := c.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(h.log, "stop") || readStatus(t, c).Result != "applied" {
		t.Fatalf("reporting retry restarted the agent: %v", h.log)
	}
}

func TestReportedAttemptLeavesBootStartupToAgentUnit(t *testing.T) {
	for _, failure := range []string{"", "apply", "prepare"} {
		for _, bootState := range []string{"stopped", "starting"} {
			t.Run(failure+"/"+bootState, func(t *testing.T) {
				c, h := fixture(t)
				h.fail = failure
				if err := c.Update(context.Background()); err != nil {
					t.Fatal(err)
				}
				before, err := os.ReadFile(filepath.Join(c.State.Root, "status.json"))
				if err != nil {
					t.Fatal(err)
				}
				h.log, h.fail = nil, ""
				h.stopped = bootState == "stopped"
				h.startingChecks = -1
				if err := c.Recover(context.Background()); err != nil {
					t.Fatal(err)
				}
				after, err := os.ReadFile(filepath.Join(c.State.Root, "status.json"))
				if err != nil || string(after) != string(before) || len(h.log) != 0 || h.stopped != (bootState == "stopped") {
					t.Fatalf("reported attempt interfered with boot or status: %v %v", h.log, err)
				}
				if err := c.Gate(context.Background()); err != nil {
					t.Fatalf("reported attempt blocked normal boot: %v", err)
				}
			})
		}
	}
}

func TestUnreportedRecoveryWaitsForSelectedAgentHealth(t *testing.T) {
	for _, scenario := range []string{"delayed-health", "deadline", "wrong-image"} {
		t.Run(scenario, func(t *testing.T) {
			c, h := fixture(t)
			if err := c.Update(context.Background()); err != nil {
				t.Fatal(err)
			}
			j, err := c.State.Active()
			if err != nil {
				t.Fatal(err)
			}
			j.Reported = false
			if err := c.State.Save(*j); err != nil {
				t.Fatal(err)
			}
			h.log, h.startingChecks = nil, 2
			if scenario == "deadline" {
				c.Config.CommandSeconds = 1
				h.startingChecks = -1
			}
			if scenario == "wrong-image" {
				h.running = oldImage
			}
			err = c.Recover(context.Background())
			if (err == nil) != (scenario == "delayed-health") {
				t.Fatalf("unexpected recovery result: %v", err)
			}
			if scenario == "deadline" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("readiness wait lost its deadline: %v", err)
			}
			if slices.Contains(h.log, "stop") || slices.Contains(h.log, "rollback") || slices.Contains(h.log, "start v0.32.0") {
				t.Fatalf("recovery disturbed a live container: %v", h.log)
			}
			if slices.Contains(h.log, "select v0.32.0") || slices.Contains(h.log, "verify-stopped") {
				t.Fatalf("recovery reselected or treated a starting container as stopped: %v", h.log)
			}
			if slices.Contains(h.log, "wait-for-startup") != (scenario != "wrong-image") {
				t.Fatalf("recovery did not delegate the selected container's readiness to systemd: %v", h.log)
			}
			j, err = c.State.Active()
			if err != nil || j.Phase != PhaseCommitted || j.Reported != (scenario == "delayed-health") {
				t.Fatalf("recovery lost the durable decision: %+v %v", j, err)
			}
			if scenario == "delayed-health" && (readStatus(t, c).Result != "applied" || slices.Contains(h.log, "suspend-watcher")) {
				t.Fatalf("delayed healthy startup suspended updates or reported failure: %v", h.log)
			}
		})
	}
}

func TestRetryRecoveryResumesVerifiedRestore(t *testing.T) {
	c, h := fixture(t)
	lock, err := c.State.Lock()
	if err != nil {
		t.Fatal(err)
	}
	j := Journal{Version: 1, AttemptID: "restore-retry", Archive: "new.tar", Previous: oldImage, Candidate: newImage, RollbackVerified: true}
	if err := c.savePhase(&j, PhaseRestoring); err != nil {
		t.Fatal(err)
	}
	_ = lock.Close()
	h.stopped, h.fail = true, "extensions"
	if err := c.Recover(context.Background()); err == nil {
		t.Fatal("failed restoration accepted")
	}
	h.fail, h.log = "", nil
	if err := c.RetryRecovery(context.Background(), j.AttemptID); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(h.log, "rollback") || slices.Contains(h.log, "verify") {
		t.Fatalf("verified restore repeated database recovery: %v", h.log)
	}
	requireOrder(t, h.log, "extensions v0.31.0", "start v0.31.0 normal")
	if readStatus(t, c).Result != "rolled-back" {
		t.Fatal("successful restoration did not report rollback")
	}
}

func TestRetryRecoveryAbortsPreparingWithoutDownSQL(t *testing.T) {
	c, h := fixture(t)
	lock, err := c.State.Lock()
	if err != nil {
		t.Fatal(err)
	}
	j := Journal{Version: 1, AttemptID: "prepare-retry", Archive: "new.tar", Previous: oldImage, Candidate: newImage, RecoveryFrom: PhasePreparing}
	if err := c.savePhase(&j, PhaseRecoveryFailed); err != nil {
		t.Fatal(err)
	}
	_ = lock.Close()
	h.stopped = true
	if err := c.RetryRecovery(context.Background(), j.AttemptID); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(h.log, "rollback") || slices.Contains(h.log, "verify") {
		t.Fatalf("preparing retry ran database recovery: %v", h.log)
	}
	requireOrder(t, h.log, "start v0.31.0 normal")
}

func TestRecoverPreservesJournalAndStopErrors(t *testing.T) {
	c, h := fixture(t)
	lock, err := c.State.Lock()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.State.write("host/active.json", map[string]string{"attempt_id": "missing-journal"}); err != nil {
		t.Fatal(err)
	}
	_ = lock.Close()
	h.stopError = errors.New("agent stop failed")
	err = c.Recover(context.Background())
	if !errors.Is(err, os.ErrNotExist) || !errors.Is(err, h.stopError) || !slices.Contains(h.log, "suspend-watcher") {
		t.Fatalf("recovery discarded a failure or left updates enabled: %v %v", err, h.log)
	}
}

func TestGatePreservesImageInspectionError(t *testing.T) {
	c, h := fixture(t)
	if err := c.Update(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.inspectError = errors.New("Podman socket unavailable")
	if err := c.Gate(context.Background()); !errors.Is(err, h.inspectError) {
		t.Fatalf("gate lost the inspection cause: %v", err)
	}
}

func TestSourceInspectionFailureRetainsPublishedArchive(t *testing.T) {
	c, h := fixture(t)
	h.failInspection = true
	if err := c.Update(context.Background()); err == nil {
		t.Fatal("source inspection failure accepted")
	}
	if _, err := os.Stat(filepath.Join(c.State.Root, "new.tar")); err != nil {
		t.Fatalf("valid archive was not retained for retry: %v", err)
	}
	if _, err := os.Stat(filepath.Join(c.State.Root, "status.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source failure was published as an archive rejection: %v", err)
	}
	if !slices.Contains(h.log, "suspend-watcher") || slices.Contains(h.log, "stop") {
		t.Fatalf("source inspection failure changed the agent or left updates enabled: %v", h.log)
	}
	h.failInspection = false
	if err := c.Update(context.Background()); err != nil {
		t.Fatal(err)
	}
	if readStatus(t, c).Result != "applied" {
		t.Fatal("retained archive could not be retried")
	}
}

func TestCanceledPreparationRestartsPreviousImage(t *testing.T) {
	c, h := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.cancelPrepare = cancel
	if err := c.Update(ctx); err != nil {
		t.Fatal(err)
	}
	requireOrder(t, h.log, "prepare", "cancel-offline-cli", "verify-stopped", "start v0.31.0 normal")
	if h.stopped || h.running != oldImage || slices.Contains(h.log, "rollback") || slices.Contains(h.log, "apply") {
		t.Fatalf("canceled preparation failed to restore the previous agent: %v", h.log)
	}
	status := readStatus(t, c)
	if status.Result != "failed" || status.ErrorStage != "migration-prepare" {
		t.Fatalf("canceled preparation lost its result: %+v", status)
	}
}

func TestPreparationCleanupFailureKeepsStartupBlocked(t *testing.T) {
	for _, failure := range []string{"cancel-offline", "verify-stopped"} {
		t.Run(failure, func(t *testing.T) {
			c, h := fixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			h.cancelPrepare = cancel
			cleanupErr := errors.New("preparation container termination is unconfirmed")
			if failure == "cancel-offline" {
				h.cancelOfflineError = cleanupErr
			} else {
				h.verifyStoppedError = cleanupErr
			}
			err := c.Update(ctx)
			if !errors.Is(err, context.Canceled) || !errors.Is(err, cleanupErr) {
				t.Fatalf("preparation or cleanup failure lost: %v", err)
			}
			j, err := c.State.Active()
			if err != nil || j.Phase != PhaseRecoveryFailed || j.RecoveryFrom != PhasePreparing || j.Reported {
				t.Fatalf("cleanup failure did not retain blocked recovery state: %+v %v", j, err)
			}
			if err := c.Gate(context.Background()); err == nil {
				t.Fatal("unconfirmed offline termination allowed normal startup")
			}
			requireOrder(t, h.log, "prepare", "cancel-offline-cli", "stop recovery-failed", "suspend-watcher")
			for _, event := range []string{"apply", "rollback", "verify", "start v0.31.0", "start v0.32.0"} {
				if slices.Contains(h.log, event) {
					t.Fatalf("cleanup failure allowed %s: %v", event, h.log)
				}
			}
			if failure == "cancel-offline" && slices.Contains(h.log, "verify-stopped") {
				t.Fatal("continued after offline cleanup failed")
			}
			if _, err := os.Stat(filepath.Join(c.State.Root, "new.tar")); err != nil {
				t.Fatalf("unresolved archive lost: %v", err)
			}
			// Once termination can be confirmed, explicit recovery must resume
			// the preparation abort without ever running down SQL.
			h.cancelOfflineError, h.verifyStoppedError, h.log = nil, nil, nil
			if err := c.RetryRecovery(context.Background(), j.AttemptID); err != nil {
				t.Fatal(err)
			}
			if slices.Contains(h.log, "rollback") || slices.Contains(h.log, "verify") || h.running != oldImage {
				t.Fatalf("preparation recovery did not safely restore the previous image: %v", h.log)
			}
		})
	}
}
