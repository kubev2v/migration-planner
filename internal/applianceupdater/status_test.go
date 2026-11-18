package applianceupdater

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

type startupInspection struct {
	image Image
	err   error
}

type startupRuntimeHost struct {
	*testHost
	inspections []startupInspection
	startError  error
	startCalls  int
}

func (h *startupRuntimeHost) Running(context.Context) (Image, error) {
	if len(h.inspections) == 0 {
		return Image{}, errors.New("unexpected additional container inspection")
	}
	result := h.inspections[0]
	h.inspections = h.inspections[1:]
	return result.image, result.err
}

func (h *startupRuntimeHost) Start(context.Context) error {
	h.startCalls++
	return h.startError
}

func TestEnsureRunningPreservesRuntimeErrors(t *testing.T) {
	inspectionErr := &os.PathError{Op: "connect", Path: "podman.sock", Err: os.ErrPermission}
	stoppedErr := errors.New("cannot inspect data consumers")
	for _, cause := range []error{inspectionErr, context.Canceled, context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			c, h := fixture(t)
			h.verifyStoppedError = stoppedErr
			runtime := &startupRuntimeHost{testHost: h, inspections: []startupInspection{{err: cause}}}
			c.Runtime = runtime
			err := c.ensureRunning(context.Background(), newImage)
			if !errors.Is(err, cause) || !errors.Is(err, stoppedErr) {
				t.Fatalf("inspection or exit-verification failure lost: %v", err)
			}
			if cause == inspectionErr {
				var pathErr *os.PathError
				if !errors.As(err, &pathErr) || pathErr != inspectionErr {
					t.Fatalf("typed Podman error lost: %v", err)
				}
			}
			if runtime.startCalls != 0 || slices.Contains(h.log, "stop") {
				t.Fatalf("uncertain container state disturbed the agent: %v", h.log)
			}
		})
	}
}

func TestEnsureRunningWaitsAndVerifiesWithoutPolling(t *testing.T) {
	inspectionErr := &os.PathError{Op: "connect", Path: "podman.sock", Err: os.ErrPermission}
	cases := []struct {
		name       string
		starting   bool
		startError error
		result     startupInspection
		wantError  error
		wantText   string
	}{
		{"stopped starts healthy", false, nil, startupInspection{image: newImage}, nil, ""},
		{"starting becomes healthy", true, nil, startupInspection{image: newImage}, nil, ""},
		{"stopped inspection error", false, nil, startupInspection{err: inspectionErr}, inspectionErr, "inspect agent after startup"},
		{"starting inspection error", true, nil, startupInspection{err: inspectionErr}, inspectionErr, "inspect agent after startup"},
		{"starting remains unhealthy", true, nil, startupInspection{image: newImage, err: errAgentStarting}, errAgentStarting, "inspect agent after startup"},
		{"wrong image after start", false, nil, startupInspection{image: oldImage}, nil, "does not match"},
		{"wrong image after wait", true, nil, startupInspection{image: oldImage}, nil, "does not match"},
		{"start cancelled", false, context.Canceled, startupInspection{}, context.Canceled, "start selected agent"},
		{"wait deadline", true, context.DeadlineExceeded, startupInspection{}, context.DeadlineExceeded, "start selected agent"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, h := fixture(t)
			h.stopped = !tc.starting
			initial := startupInspection{err: errors.New("container is stopped")}
			if tc.starting {
				initial = startupInspection{image: newImage, err: errAgentStarting}
			}
			runtime := &startupRuntimeHost{
				testHost: h, startError: tc.startError,
				inspections: []startupInspection{initial},
			}
			if tc.startError == nil {
				runtime.inspections = append(runtime.inspections, tc.result)
			}
			c.Runtime = runtime
			err := c.ensureRunning(context.Background(), newImage)
			if tc.wantText == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantText) {
				t.Fatalf("expected %q error, got %v", tc.wantText, err)
			}
			if tc.wantError != nil && !errors.Is(err, tc.wantError) {
				t.Fatalf("underlying startup failure lost: %v", err)
			}
			if runtime.startCalls != 1 || len(runtime.inspections) != 0 {
				t.Fatalf("startup did not wait once and inspect once: %+v", runtime)
			}
			if tc.starting && len(h.log) != 0 {
				t.Fatalf("waiting stopped or reselected the existing container: %v", h.log)
			}
			if !tc.starting {
				requireOrder(t, h.log, "verify-stopped", "stop", "select v0.32.0")
			}
		})
	}
}

// Inject a failure at either startup reporting call site, after the journal
// has been read successfully, so persistence failures cannot hide its cause.
type startupFailureHost struct {
	*testHost
	metadata      bool
	runningChecks int
	cause         error
	beforeFailure func()
}

func (h *startupFailureHost) Running(ctx context.Context) (Image, error) {
	if !h.metadata {
		return h.testHost.Running(ctx)
	}
	h.runningChecks++
	if h.runningChecks == 1 {
		return newImage, nil
	}
	h.beforeFailure()
	return Image{}, h.cause
}

func (h *startupFailureHost) Start(context.Context) error {
	h.beforeFailure()
	return h.cause
}

func TestStartupFailurePreservesPersistenceErrors(t *testing.T) {
	for _, stage := range []string{"restart", "metadata"} {
		for _, persistence := range []string{"journal", "status", "both"} {
			t.Run(stage+"/"+persistence, func(t *testing.T) {
				c, h := fixture(t)
				lock, err := c.State.Lock()
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = lock.Close() })
				j := Journal{
					Version: 1, AttemptID: "report-failure", Archive: "new.tar",
					Previous: oldImage, Candidate: newImage, Phase: PhaseCommitted,
					CommitTime: time.Now().UTC(), Reported: true,
				}
				if err := c.State.Save(j); err != nil {
					t.Fatal(err)
				}
				h.selected, h.stopped = newImage, true
				cause := errors.New("injected startup or metadata failure")
				c.Runtime = &startupFailureHost{
					testHost: h, metadata: stage == "metadata", cause: cause,
					beforeFailure: func() {
						if persistence != "status" {
							pointer := filepath.Join(c.State.Root, "host/active.json")
							if err := os.Remove(pointer); err != nil {
								t.Fatal(err)
							}
							if err := os.Mkdir(pointer, 0700); err != nil {
								t.Fatal(err)
							}
						}
						if persistence != "journal" {
							if err := os.Mkdir(filepath.Join(c.State.Root, "status.json"), 0700); err != nil {
								t.Fatal(err)
							}
						}
					},
				}
				err = c.finishAttempt(context.Background(), &j)
				if !errors.Is(err, cause) {
					t.Fatalf("original failure lost: %v", err)
				}
				if persistence != "status" && !strings.Contains(err.Error(), "save startup failure journal") {
					t.Fatalf("journal persistence failure lost: %v", err)
				}
				if persistence != "journal" && !strings.Contains(err.Error(), "publish startup failure status") {
					t.Fatalf("status persistence failure lost: %v", err)
				}
				var pathErr *os.PathError
				var linkErr *os.LinkError
				if !errors.As(err, &pathErr) && !errors.As(err, &linkErr) {
					t.Fatalf("underlying persistence error lost: %v", err)
				}
				var durable Journal
				if err := c.State.read("host/attempts/"+j.AttemptID+".json", &durable); err != nil {
					t.Fatal(err)
				}
				if durable.Phase != PhaseCommitted || durable.Reported || !durable.CommitTime.Equal(j.CommitTime) {
					t.Fatalf("failure changed the durable decision or lost retry state: %+v", durable)
				}
				if persistence == "journal" {
					status := readStatus(t, c)
					if status.Result != "failed" || status.ErrorStage != stage {
						t.Fatalf("journal failure prevented status publication: %+v", status)
					}
				}
			})
		}
	}
}
