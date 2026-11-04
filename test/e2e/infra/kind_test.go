package infra

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kubev2v/migration-planner/test/e2e/config"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
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

func TestWaitForAllDeploymentsReady(t *testing.T) {
	replicas := int32(2)
	zero := int32(0)
	k := &KindInfraManager{
		cfg: config.InfraConfig{Namespace: "default", DeploymentReadyTimeout: time.Second},
		clientset: fake.NewClientset(
			&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "planner", Namespace: "default"},
				Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
				Status:     appsv1.DeploymentStatus{ReadyReplicas: 2},
			},
			&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "scaled-down", Namespace: "default"},
				Spec:       appsv1.DeploymentSpec{Replicas: &zero},
			},
		),
	}
	if err := k.waitForAllDeployments(); err != nil {
		t.Fatalf("ready deployments should not time out: %v", err)
	}
}

func TestWaitForAllDeploymentsTimeoutReportsAllUnready(t *testing.T) {
	replicas := int32(2)
	k := &KindInfraManager{
		cfg: config.InfraConfig{Namespace: "default", DeploymentReadyTimeout: 20 * time.Millisecond},
		clientset: fake.NewClientset(
			&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "planner", Namespace: "default"},
				Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
				Status:     appsv1.DeploymentStatus{ReadyReplicas: 1},
			},
			&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "sizer", Namespace: "default"},
			},
			&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "ready", Namespace: "default"},
				Status:     appsv1.DeploymentStatus{ReadyReplicas: 1},
			},
		),
	}
	err := k.waitForAllDeployments()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected the configured deadline to expire, got %v", err)
	}
	for _, want := range []string{"20ms", "planner (1/2 ready)", "sizer (0/1 ready)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("timeout %q does not include %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "ready (1/1 ready)") {
		t.Errorf("timeout includes a ready deployment: %v", err)
	}
}

func TestWaitForAllDeploymentsListFailurePreservesDeadline(t *testing.T) {
	listErr := errors.New("deployment API unavailable")
	clientset := fake.NewClientset()
	clientset.PrependReactor("list", "deployments", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, listErr
	})
	k := &KindInfraManager{
		cfg:       config.InfraConfig{Namespace: "default", DeploymentReadyTimeout: 20 * time.Millisecond},
		clientset: clientset,
	}

	err := k.waitForAllDeployments()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected the configured deadline to expire, got %v", err)
	}
	if !errors.Is(err, listErr) {
		t.Fatalf("expected the deployment listing error to be preserved, got %v", err)
	}
	for _, want := range []string{"timed out listing deployments", "last unready: no deployment status observed", listErr.Error()} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("timeout %q does not include %q", err, want)
		}
	}
}
