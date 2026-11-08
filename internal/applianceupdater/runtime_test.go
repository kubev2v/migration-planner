package applianceupdater

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"go.podman.io/podman/v6/pkg/specgen"
)

// Exercise the actual SDK HTTP requests and decoding, without requiring Podman
// or changing the developer's containers.
type podmanAPI struct {
	mu             sync.Mutex
	specs          []specgen.SpecGenerator
	removed        []string
	inventory      []map[string]any
	inspection     map[string]any
	inspectFailure bool
	output         string
	exitCode       int
	waitFailure    bool
}

func testPodman(t *testing.T, config Config, api *podmanAPI) *Host {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api.mu.Lock()
		defer api.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		if strings.HasSuffix(path, "/_ping") {
			w.Header().Set("Libpod-API-Version", "5.0.0")
			_, _ = w.Write([]byte("OK"))
			return
		}
		_, path, _ = strings.Cut(path, "/libpod")
		respond := func(value any) {
			if err := json.NewEncoder(w).Encode(value); err != nil {
				t.Error(err)
			}
		}
		switch {
		case path == "/containers/json":
			respond(api.inventory)
		case path == "/containers/create":
			var spec specgen.SpecGenerator
			if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
				t.Error(err)
			}
			api.specs = append(api.specs, spec)
			respond(map[string]any{"Id": "offline-container"})
		case strings.HasPrefix(path, "/containers/") && r.Method == http.MethodDelete:
			if r.URL.Query().Get("force") != "true" || r.URL.Query().Get("ignore") != "true" {
				t.Error("cleanup must force removal and tolerate absence")
			}
			api.removed = append(api.removed, strings.TrimPrefix(path, "/containers/"))
			respond([]any{})
		case strings.HasSuffix(path, "/start"):
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(path, "/wait"):
			if api.waitFailure {
				http.Error(w, `{"message":"wait failed"}`, 500)
				return
			}
			respond(api.exitCode)
		case strings.HasSuffix(path, "/logs"):
			output := api.output
			if output == "" && len(api.specs) > 0 && api.specs[len(api.specs)-1].Entrypoint[0] == "/app/agent" {
				args := api.specs[len(api.specs)-1].Command
				output = fmt.Sprintf(`{"attempt_id":%q,"operation":%q,"dry_run":false,"databases":[]}`, args[5], args[1])
			}
			header := make([]byte, 8)
			header[0] = 1
			binary.BigEndian.PutUint32(header[4:], uint32(len(output)))
			_, _ = w.Write(header)
			_, _ = w.Write([]byte(output))
		case strings.HasPrefix(path, "/containers/") && strings.HasSuffix(path, "/json"):
			if api.inspectFailure {
				http.Error(w, `{"message":"inspect failed"}`, 500)
				return
			}
			respond(api.inspection)
		case strings.HasPrefix(path, "/images/") && strings.HasSuffix(path, "/json"):
			respond(map[string]any{"Id": newImage.ID, "Digest": newImage.Digest, "Labels": map[string]string{"org.opencontainers.image.version": newImage.Version}})
		case strings.HasSuffix(path, "/tag"):
			if r.URL.Query().Get("repo") != "localhost/appliance" || r.URL.Query().Get("tag") != "candidate" {
				t.Error("incorrect image tag")
			}
			w.WriteHeader(http.StatusCreated)
		default:
			t.Errorf("unexpected Podman request: %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	h, err := NewHost(config)
	if err != nil {
		t.Fatal(err)
	}
	h.podmanSocket = strings.Replace(server.URL, "http://", "tcp://", 1)
	return h
}

func TestPinnedOfflineMigrationContainers(t *testing.T) {
	c, _ := fixture(t)
	api := &podmanAPI{}
	h := testPodman(t, c.Config, api)
	attempt := Journal{AttemptID: "offline-test", Previous: oldImage, Candidate: newImage}
	for _, op := range []string{"prepare", "apply", "rollback", "verify"} {
		if err := h.Migrate(context.Background(), attempt, op); err != nil {
			t.Fatal(err)
		}
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	if len(api.specs) != 4 || len(api.removed) != 8 {
		t.Fatalf("missing runs or cleanup: %+v", api)
	}
	for i, spec := range api.specs {
		if spec.Image != newImage.ID || !reflect.DeepEqual(spec.Entrypoint, []string{"/app/agent"}) || spec.NetNS.NSMode != specgen.NoNetwork || spec.UserNS.String() != "keep-id:uid=1001" {
			t.Fatalf("unsafe offline container: %+v", spec)
		}
		if len(spec.Mounts) != 1 || spec.Mounts[0].Source != c.Config.DataDir || spec.Mounts[0].Destination != "/var/lib/agent" || !reflect.DeepEqual(spec.Mounts[0].Options, []string{"rw", "rbind", "Z"}) {
			t.Fatalf("incorrect data mount: %+v", spec.Mounts)
		}
		if api.removed[2*i] != "oma-upgrade-offline-test" || api.removed[2*i+1] != "offline-container" {
			t.Fatal("interrupted or completed container not removed")
		}
	}
	if !strings.HasSuffix(strings.Join(api.specs[3].Command, " "), "--target baseline") {
		t.Fatal("recovery omitted baseline verification")
	}
}

func TestOfflineContainerFailureCleanup(t *testing.T) {
	for _, scenario := range []string{"exit-code", "wait-error", "invalid-result", "output-limit", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			c, _ := fixture(t)
			api := &podmanAPI{}
			switch scenario {
			case "exit-code":
				api.exitCode = 3
			case "wait-error":
				api.waitFailure = true
			case "invalid-result":
				api.output = `{"attempt_id":"another","operation":"apply","databases":[]}`
			case "output-limit":
				api.output = strings.Repeat("x", 1024*1024)
			}
			h := testPodman(t, c.Config, api)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "cancelled" {
				cancel()
			}
			err := h.Migrate(ctx, Journal{AttemptID: "failure", Previous: oldImage, Candidate: newImage}, "apply")
			if err == nil {
				t.Fatal("failed migration accepted")
			}
			api.mu.Lock()
			defer api.mu.Unlock()
			if scenario != "cancelled" && (len(api.removed) != 2 || api.removed[1] != "offline-container") {
				t.Fatal("failed container not removed")
			}
		})
	}
}

func TestExtensionCompatibilityWithoutPersistentWrites(t *testing.T) {
	for _, scenario := range []string{"matching", "different", "missing", "malformed", "missing-sqlite"} {
		t.Run(scenario, func(t *testing.T) {
			c, _ := fixture(t)
			extension := []byte("installed-extension")
			path := filepath.Join(c.Config.DataDir, "sqlite_scanner.duckdb_extension")
			if scenario != "missing" {
				if err := os.WriteFile(path, extension, 0600); err != nil {
					t.Fatal(err)
				}
			}
			response := fmt.Sprintf("%x  /app/extensions/sqlite_scanner.duckdb_extension\n", sha256.Sum256(extension))
			switch scenario {
			case "different":
				response = fmt.Sprintf("%x  /app/extensions/sqlite_scanner.duckdb_extension\n", sha256.Sum256([]byte("candidate-extension")))
			case "malformed":
				response = "invalid checksums"
			case "missing-sqlite":
				response = " "
			}
			api := &podmanAPI{output: response}
			h := testPodman(t, c.Config, api)
			err := h.CheckExtensions(context.Background(), newImage)
			if (err == nil) != (scenario == "matching") {
				t.Fatalf("incorrect extension result: %v", err)
			}
			if scenario != "missing" {
				after, err := os.ReadFile(path)
				if err != nil || string(after) != string(extension) {
					t.Fatal("probe modified installed assets")
				}
			}
			api.mu.Lock()
			defer api.mu.Unlock()
			spec := api.specs[0]
			if spec.ReadOnlyFilesystem == nil || !*spec.ReadOnlyFilesystem || len(spec.Mounts) != 0 || spec.Image != newImage.ID || spec.NetNS.NSMode != specgen.NoNetwork {
				t.Fatalf("unsafe extension probe: %+v", spec)
			}
		})
	}
}

func TestStopAndMigrationVerifyAllDataConsumers(t *testing.T) {
	for _, scenario := range []string{"none", "exited", "agent-running", "shared-data", "ancestor-mount", "child-mount", "symlink-mount", "unrelated", "paused", "unknown", "inspect-failure", "stop-failure"} {
		t.Run(scenario, func(t *testing.T) {
			c, _ := fixture(t)
			folder := t.TempDir()
			source := c.Config.DataDir
			name := "another-container"
			state := map[string]any{"Running": true, "Pid": 123, "Status": "running"}
			switch scenario {
			case "exited":
				state = map[string]any{"Running": false, "Pid": 0, "Status": "exited"}
			case "agent-running":
				name = "planner-agent"
			case "ancestor-mount":
				source = filepath.Dir(source)
			case "child-mount":
				source = filepath.Join(source, "child")
				if err := os.Mkdir(source, 0700); err != nil {
					t.Fatal(err)
				}
			case "symlink-mount":
				link := filepath.Join(folder, "alias")
				if err := os.Symlink(source, link); err != nil {
					t.Fatal(err)
				}
				source = link
			case "unrelated":
				source = t.TempDir()
			case "paused":
				state = map[string]any{"Running": false, "Pid": 123, "Status": "paused"}
			case "unknown":
				state = map[string]any{}
			}
			api := &podmanAPI{inspection: map[string]any{"Name": name, "State": state, "Mounts": []map[string]string{{"Type": "bind", "Source": source}}}, inspectFailure: scenario == "inspect-failure"}
			if scenario != "none" {
				api.inventory = []map[string]any{{"Id": strings.Repeat("1", 64)}}
			}
			script := "#!/bin/sh\nexit 0\n"
			if scenario == "stop-failure" {
				script = "#!/bin/sh\nexit 1\n"
			}
			if err := os.WriteFile(filepath.Join(folder, "systemctl"), []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", folder+string(os.PathListSeparator)+os.Getenv("PATH"))
			h := testPodman(t, c.Config, api)
			allowed := scenario == "none" || scenario == "exited" || scenario == "unrelated"
			err := h.Stop(context.Background())
			if (err == nil) != allowed {
				t.Fatalf("incorrect stop verification: %v", err)
			}
			if scenario != "stop-failure" {
				err = h.Migrate(context.Background(), Journal{AttemptID: "stop-test", Previous: oldImage, Candidate: newImage}, "prepare")
				if (err == nil) != allowed {
					t.Fatalf("migration bypassed consumer verification: %v", err)
				}
			}
			api.mu.Lock()
			defer api.mu.Unlock()
			if !allowed && len(api.specs) > 0 {
				t.Fatal("CLI ran while stop could not be verified")
			}
		})
	}
}

func TestRunningRequiresHealthyContainerAndPinnedImage(t *testing.T) {
	for _, scenario := range []string{"healthy", "starting", "unhealthy", "missing-health", "stopped"} {
		t.Run(scenario, func(t *testing.T) {
			c, _ := fixture(t)
			state := map[string]any{"Running": scenario != "stopped"}
			if scenario != "missing-health" {
				state["Health"] = map[string]string{"Status": scenario}
			}
			api := &podmanAPI{inspection: map[string]any{"Image": newImage.ID, "State": state}}
			h := testPodman(t, c.Config, api)
			image, err := h.Running(context.Background())
			if scenario == "healthy" {
				if err != nil || image != newImage {
					t.Fatalf("healthy image not confirmed: %+v %v", image, err)
				}
			} else if err == nil {
				t.Fatal("container accepted without healthy startup")
			}
		})
	}
}

func TestPinUsesPodmanImageTagAPI(t *testing.T) {
	c, _ := fixture(t)
	h := testPodman(t, c.Config, &podmanAPI{})
	if err := h.Pin(context.Background(), newImage, "localhost/appliance:candidate"); err != nil {
		t.Fatal(err)
	}
}
