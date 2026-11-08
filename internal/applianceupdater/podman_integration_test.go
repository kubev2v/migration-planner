package applianceupdater

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.podman.io/podman/v6/pkg/bindings/containers"
	"go.podman.io/podman/v6/pkg/specgen"
)

// Opt in with a local socket and an existing image containing /bin/sh and id.
// This test creates and removes its own containers without touching the agent.
func TestPodmanSDKIntegration(t *testing.T) {
	socket, imageRef := os.Getenv("APPLIANCE_UPDATER_TEST_PODMAN_SOCKET"), os.Getenv("APPLIANCE_UPDATER_TEST_IMAGE")
	if socket == "" || imageRef == "" {
		t.Skip("requires a local Podman socket and test image")
	}
	c, _ := fixture(t)
	c.Config.CommandSeconds = 60
	h, err := NewHost(c.Config)
	if err != nil {
		t.Fatal(err)
	}
	h.podmanSocket = socket
	ctx := context.Background()
	image, err := h.Inspect(ctx, imageRef)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"success", "failure"} {
		t.Run(scenario, func(t *testing.T) {
			spec := specgen.NewSpecGenerator(image.ID, false)
			spec.Name = "oma-updater-sdk-test-" + uuid.NewString()
			spec.Entrypoint = []string{"/bin/sh"}
			spec.Command = []string{"-c", `printf 'sdk-probe:%s' "$(id -u)"`}
			spec.UserNS = specgen.Namespace{NSMode: specgen.KeepID, Value: "uid=1001"}
			readOnly := true
			spec.ReadOnlyFilesystem = &readOnly
			if scenario == "failure" {
				spec.Command = []string{"-c", "exit 3"}
			}
			output, err := h.runContainer(ctx, spec)
			if scenario == "success" && (err != nil || string(output) != "sdk-probe:1001") {
				t.Fatalf("unexpected offline result: %q %v", output, err)
			}
			if scenario == "failure" && (err == nil || !strings.Contains(err.Error(), "exited with code 3")) {
				t.Fatalf("expected container exit code 3: %v", err)
			}
			api, close, err := h.connectPodman(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer close()
			exists, err := containers.Exists(api, spec.Name, nil)
			if err != nil || exists {
				t.Fatalf("container cleanup failed: exists=%v err=%v", exists, err)
			}
		})
	}
}
