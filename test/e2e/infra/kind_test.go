package infra

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/kubev2v/migration-planner/test/e2e/config"
)

// stubExec replaces execCommandContext with a fake that runs TestHelperProcess
// and exits with the code returned by decide for each (name, args) invocation.
func stubExec(t *testing.T, decide func(name string, args []string) int) {
	t.Helper()
	orig := execCommandContext
	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		code := decide(name, args)
		helperArgs := []string{"-test.run=TestHelperProcess", "--", strconv.Itoa(code)}
		cmd := exec.CommandContext(ctx, os.Args[0], helperArgs...)
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
		return cmd
	}
	t.Cleanup(func() { execCommandContext = orig })
}

// TestHelperProcess is not a real test; it is the child process spawned by
// stubExec. It exits with the code passed as its last argument.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	code, err := strconv.Atoi(os.Args[len(os.Args)-1])
	if err != nil {
		code = 1
	}
	os.Exit(code)
}

func TestBuildVcsimImageError(t *testing.T) {
	stubExec(t, func(name string, args []string) int { return 1 })

	k := &KindInfraManager{}
	if _, err := k.buildVcsimImage(context.Background()); err == nil {
		t.Fatal("expected error when docker build fails, got nil")
	}
}

func TestLoadImageError(t *testing.T) {
	// docker image inspect succeeds so we reach the kind load step, which fails.
	stubExec(t, func(name string, args []string) int {
		if name == "kind" {
			return 1
		}
		return 0
	})

	k := &KindInfraManager{}
	if err := k.loadImage(context.Background(), e2eVcsimImage); err == nil {
		t.Fatal("expected error when kind load fails, got nil")
	}
}

func TestVcsimTemplateParams(t *testing.T) {
	v := config.VcsimInstance{Name: "vcsim1", Port: 8989, Username: "user", Password: "pass"}

	params := vcsimTemplateParams(v, e2eVcsimImage)

	if got := params["VCSIM_IMAGE"]; got != e2eVcsimImage {
		t.Errorf("VCSIM_IMAGE = %q, want %q", got, e2eVcsimImage)
	}
	if got := params["VCSIM_IMAGE_PULL_POLICY"]; got != "Never" {
		t.Errorf("VCSIM_IMAGE_PULL_POLICY = %q, want %q", got, "Never")
	}
	if got := params["APP_NAME"]; got != "vcsim1" {
		t.Errorf("APP_NAME = %q, want %q", got, "vcsim1")
	}
	if got := params["PORT"]; got != "8989" {
		t.Errorf("PORT = %q, want %q", got, "8989")
	}
	if got := params["USERNAME"]; got != "user" {
		t.Errorf("USERNAME = %q, want %q", got, "user")
	}
	if got := params["PASSWORD"]; got != "pass" {
		t.Errorf("PASSWORD = %q, want %q", got, "pass")
	}
}
