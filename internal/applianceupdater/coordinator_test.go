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

var oldImage = Image{"sha256:" + strings.Repeat("a", 64), "sha256:old-digest", "source-release"}
var newImage = Image{"sha256:" + strings.Repeat("b", 64), "sha256:new-digest", "candidate-release"}

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
}

func (h *testHost) Running(context.Context) (Image, error) {
	if h.stopped || h.failInspection {
		return Image{}, errors.New("container unavailable")
	}
	return h.running, nil
}
func (h *testHost) Inspect(_ context.Context, _ string) (Image, error) { return h.selected, nil }
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
func (h *testHost) VerifyStopped(context.Context) error {
	if !h.stopped {
		return errors.New("container is still running")
	}
	return nil
}
func (h *testHost) Stop(context.Context) error {
	h.log = append(h.log, "stop")
	if j, err := h.c.State.Active(); err == nil && j != nil {
		h.log = append(h.log, "stop "+string(j.Phase))
	}
	if h.fail == "stop" {
		return errors.New("container still running")
	}
	h.stopped = true
	return nil
}
func (h *testHost) Start(ctx context.Context) error {
	if err := h.c.Gate(ctx); err != nil {
		return err
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
func (h *testHost) Migrate(_ context.Context, j Journal, operation string) error {
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
	h.log = append(h.log, operation)
	if h.fail == operation {
		return errors.New("injected CLI failure")
	}
	return nil
}
func (h *testHost) Suspend(context.Context) error {
	h.log = append(h.log, "suspend-watcher")
	return nil
}

func (h *testHost) CancelOffline(_ context.Context, _ Journal) error {
	h.log = append(h.log, "cancel-offline-cli")
	if h.fail == "cancel-offline" {
		return errors.New("offline container cleanup failed")
	}
	return nil
}

func fixture(t *testing.T) (*Coordinator, *testHost) {
	t.Helper()
	folder := t.TempDir()
	cfg := Config{DataDir: folder, ImageTag: "localhost/appliance:latest", EnvironmentFile: filepath.Join(folder, "upgrade.env"), CommandSeconds: 5, Transitions: []Transition{{oldImage.ID, newImage.ID}}}
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
	requireOrder(t, h.log, "stop", "prepare", "apply", "select candidate-release", "start candidate-release normal")
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
			requireOrder(t, h.log, "apply", "rollback", "verify", "select source-release", "start source-release normal")
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
			if slices.Contains(h.log, "start source-release") {
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

func TestUnsupportedTransitionsRejectBeforeDowntime(t *testing.T) {
	for _, unsupported := range []string{"policy", "extensions"} {
		t.Run(unsupported, func(t *testing.T) {
			c, h := fixture(t)
			if unsupported == "policy" {
				c.Config.Transitions = nil
			} else {
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
			j := Journal{Version: 1, AttemptID: "reboot", Archive: "new.tar", Previous: oldImage, Candidate: newImage, CommitTime: time.Now().UTC()}
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
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if phase == "prepared" || phase == "applying" || phase == "rolling-back" {
				requireOrder(t, h.log, "rollback", "verify", "start source-release")
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
	if err := c.Gate(context.Background()); err == nil {
		t.Fatal("old mutable tag bypassed committed candidate selection")
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
	requireOrder(t, h.log, "rollback", "verify", "start source-release normal")
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
	if slices.Contains(h.log, "prepare") || slices.Contains(h.log, "apply") {
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
	requireOrder(t, h.log, "stop", "prepare", "select source-release", "start source-release normal")
	if slices.Contains(h.log, "apply") || slices.Contains(h.log, "rollback") || readStatus(t, c).ErrorStage != "migration-prepare" {
		t.Fatalf("preparation failure ran migrations: %v", h.log)
	}
}

func TestUncertainContainerStateDoesNotRestartCommittedAgent(t *testing.T) {
	c, h := fixture(t)
	if err := c.Update(context.Background()); err != nil {
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
