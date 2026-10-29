package image

import (
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Exercise scripts decoded from the actual Butane output, including Go template escaping.
func ignitionFiles(t *testing.T) (map[string]string, []string) {
	t.Helper()
	b := NewImageBuilder(uuid.New())
	b.Template = "../../" + defaultTemplate
	out, err := b.generateIgnition()
	if err != nil {
		t.Fatal(err)
	}
	var ignition struct {
		Storage struct {
			Files []struct {
				Path     string
				Contents struct{ Source, Compression string }
			}
			Links []struct{ Path string }
		}
	}
	if err := json.Unmarshal([]byte(out), &ignition); err != nil {
		t.Fatal(err)
	}
	files := make(map[string]string)
	for _, f := range ignition.Storage.Files {
		header, payload, ok := strings.Cut(f.Contents.Source, ",")
		if !ok {
			t.Fatalf("invalid data URL for %s", f.Path)
		}
		var data []byte
		if strings.HasSuffix(header, ";base64") {
			data, err = base64.StdEncoding.DecodeString(payload)
		} else {
			var decoded string
			decoded, err = url.PathUnescape(payload)
			data = []byte(decoded)
		}
		if err != nil {
			t.Fatal(err)
		}
		if f.Contents.Compression == "gzip" {
			r, err := gzip.NewReader(strings.NewReader(string(data)))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = r.Close() })
			data, err = io.ReadAll(r)
			if err != nil {
				t.Fatal(err)
			}
		}
		files[f.Path] = string(data)
	}
	var links []string
	for _, link := range ignition.Storage.Links {
		links = append(links, link.Path)
	}
	return files, links
}

func TestIgnitionTemplateAgentUpdate(t *testing.T) {
	files, links := ignitionFiles(t)
	quadlet := files["/home/core/.config/containers/systemd/planner-agent.container"]
	for _, want := range []string{
		"HealthCmd=curl -fsk https://localhost:3333/api/v2/version",
		"HealthInterval=10s", "HealthRetries=6", "HealthStartPeriod=30s",
		"Notify=healthy", "TimeoutStartSec=300",
		"Volume=/var/lib/data/agent:/var/lib/agent:Z",
	} {
		if !strings.Contains(quadlet, want) {
			t.Errorf("Quadlet is missing %q", want)
		}
	}
	if !strings.Contains(files["/home/core/.config/systemd/user/planner-agent-image-update.path"],
		"PathExistsGlob=/var/lib/data/agent/updates/*.tar") {
		t.Error("path unit must watch only published archives")
	}
	for _, link := range links {
		if strings.Contains(link, "podman-auto-update.timer") {
			t.Error("the daily podman auto-update timer must not be enabled")
		}
	}
}

type updateImage struct {
	Digest  string `json:"digest"`
	Version string `json:"version"`
}

type successfulUpdate struct {
	updateImage
	Time string `json:"time"`
}

type updateStatus struct {
	File                 string            `json:"file"`
	Result               string            `json:"result"`
	Time                 string            `json:"time"`
	Error                string            `json:"error"`
	ErrorStage           string            `json:"error_stage"`
	RunningImage         *updateImage      `json:"running_image"`
	LastSuccessfulUpdate *successfulUpdate `json:"last_successful_update"`
}

type updateFixture struct {
	t       *testing.T
	root    string
	updates string
	scripts map[string]string
	env     []string
}

const previousStatus = `{"file":"old.tar","result":"applied","time":"2026-01-01T00:00:00Z","running_image":{"digest":"sha256:old","version":"v1"},"last_successful_update":{"digest":"sha256:old","version":"v1","time":"2026-01-01T00:00:00Z"}}`

func newUpdateFixture(t *testing.T) *updateFixture {
	t.Helper()
	jq, err := exec.LookPath("jq")
	if err != nil {
		t.Fatal("host update tests require jq:", err)
	}
	mv, err := exec.LookPath("mv")
	if err != nil {
		t.Fatal(err)
	}
	files, _ := ignitionFiles(t)
	f := &updateFixture{t: t, root: t.TempDir(), scripts: make(map[string]string)}
	f.updates = filepath.Join(f.root, "agent", "updates")
	bin := filepath.Join(f.root, "bin")
	for _, dir := range []string{filepath.Join(f.updates, ".staging"), bin} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"update", "load"} {
		path := filepath.Join(f.root, name+".sh")
		script := files["/usr/local/sbin/"+name+"-planner-agent-image.sh"]
		if script == "" {
			t.Fatal("missing script:", name)
		}
		script = strings.ReplaceAll(script, "/var/lib/data", f.root)
		script = strings.ReplaceAll(script, "/run/media/iso/images/migration-planner-agent.tar", filepath.Join(f.root, "iso.tar"))
		f.write(path, script, 0o755)
		f.scripts[name] = path
	}
	f.write(filepath.Join(bin, "skopeo"), `#!/bin/bash
printf 'skopeo %s\n' "$*" >> "$COMMAND_LOG"
printf '%s' "${LOAD_STDERR:-}" >&2
exit "${LOAD_EXIT:-0}"
`, 0o755)
	f.write(filepath.Join(bin, "podman"), `#!/bin/bash
printf 'podman %s\n' "$*" >> "$COMMAND_LOG"
case "$1 $2" in
  'auto-update --format')
    printf '%s' "${UPDATE_STDERR:-}" >&2
    printf '%s\n' "$UPDATE_REPORT"
    exit "${UPDATE_EXIT:-0}" ;;
  'container inspect')
    printf '%s' "${INSPECT_STDERR:-}" >&2
    [ "${INSPECT_EXIT:-0}" = 0 ] || exit 1
    printf '[{"Image":"sha256:running","State":{"Running":%s}}]\n' "${RUNNING:-true}" ;;
  'image inspect')
    [ "$3" = sha256:running ] || exit 1
    printf '%s\n' "$IMAGE_METADATA" ;;
  'image exists') exit 0 ;;
  'image prune') exit 0 ;;
  *) exit 1 ;;
esac
`, 0o755)
	// A partial encoder write and a failed rename model publication failures without a full disk.
	f.write(filepath.Join(bin, "jq"), `#!/bin/bash
if [ "$1" = -n ] && [ "${PUBLISH_FAIL:-0}" = 1 ]; then
  printf '{"file":'
  exit 1
fi
exec "$REAL_JQ" "$@"
`, 0o755)
	f.write(filepath.Join(bin, "mv"), `#!/bin/bash
[ "${RENAME_FAIL:-0}" = 0 ] || exit 1
exec "$REAL_MV" "$@"
`, 0o755)
	f.env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"REAL_JQ="+jq, "REAL_MV="+mv, "COMMAND_LOG="+filepath.Join(f.root, "commands"),
		`UPDATE_REPORT=[{"Unit":"planner-agent.service","Updated":"true"}]`,
		`IMAGE_METADATA=[{"Digest":"sha256:new","Labels":{"org.opencontainers.image.version":"v2"}}]`)
	return f
}

func (f *updateFixture) write(path, content string, mode os.FileMode) {
	f.t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		f.t.Fatal(err)
	}
}

func (f *updateFixture) run(script string, wantFailure bool, env ...string) {
	f.t.Helper()
	cmd := exec.Command("bash", f.scripts[script])
	cmd.Env = append(append([]string{}, f.env...), env...)
	out, err := cmd.CombinedOutput()
	if (err != nil) != wantFailure {
		f.t.Fatalf("%s: error = %v, want failure = %t\n%s", script, err, wantFailure, out)
	}
}

func (f *updateFixture) read(path string) string {
	f.t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		f.t.Fatal(err)
	}
	return string(b)
}

func (f *updateFixture) status() updateStatus {
	f.t.Helper()
	var status updateStatus
	if err := json.Unmarshal([]byte(f.read(filepath.Join(f.updates, "status.json"))), &status); err != nil {
		f.t.Fatal(err)
	}
	if _, err := time.Parse(time.RFC3339, status.Time); err != nil {
		f.t.Fatal("invalid attempt timestamp:", err)
	}
	return status
}

func TestAgentUpdateOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name, result, stage string
		env                 []string
	}{
		{name: "applied", result: "applied"},
		{name: "applied with stderr", result: "applied", env: []string{"LOAD_STDERR=copy progress", "UPDATE_STDERR=restart diagnostic"}},
		{name: "load failure", result: "failed", stage: "load", env: []string{"LOAD_EXIT=1"}},
		{name: "rollback", result: "rolled-back", env: []string{`UPDATE_REPORT=[{"Unit":"planner-agent.service","Updated":"rolled back"}]`}},
		{name: "unchanged", result: "unchanged", env: []string{`UPDATE_REPORT=[{"Unit":"planner-agent.service","Updated":"false"}]`}},
		{name: "restart failure", result: "failed", stage: "restart", env: []string{`UPDATE_REPORT=[{"Unit":"planner-agent.service","Updated":"failed"}]`}},
		{name: "nonzero despite success report", result: "failed", stage: "restart", env: []string{"UPDATE_EXIT=1"}},
		{name: "unrelated success", result: "failed", stage: "restart", env: []string{`UPDATE_REPORT=[{"Unit":"planner-agent.service","Updated":"failed"},{"Unit":"other.service","Updated":"true"}]`}},
		{name: "unrelated unchanged", result: "applied", env: []string{`UPDATE_REPORT=[{"Unit":"planner-agent.service","Updated":"true"},{"Unit":"other.service","Updated":"false"}]`}},
		{name: "missing report", result: "failed", stage: "report", env: []string{`UPDATE_REPORT=[]`}},
		{name: "duplicate reports", result: "failed", stage: "report", env: []string{`UPDATE_REPORT=[{"Unit":"planner-agent.service","Updated":"true"},{"Unit":"planner-agent.service","Updated":"true"}]`}},
		{name: "malformed report", result: "failed", stage: "report", env: []string{`UPDATE_REPORT=invalid`}},
		{name: "unexpected result", result: "failed", stage: "report", env: []string{`UPDATE_REPORT=[{"Unit":"planner-agent.service","Updated":"pending"}]`}},
		{name: "missing metadata", result: "failed", stage: "metadata", env: []string{`IMAGE_METADATA=[{}]`}},
		{name: "missing version", result: "failed", stage: "metadata", env: []string{`IMAGE_METADATA=[{"Digest":"sha256:new"}]`}},
		{name: "inspect failure", result: "failed", stage: "metadata", env: []string{"INSPECT_EXIT=1"}},
		{name: "container stopped", result: "failed", stage: "metadata", env: []string{"RUNNING=false"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newUpdateFixture(t)
			f.write(filepath.Join(f.updates, "status.json"), previousStatus, 0o644)
			f.write(filepath.Join(f.updates, "new.tar"), "archive", 0o644)
			f.run("update", false, tc.env...)
			status := f.status()
			if status.Result != tc.result || status.File != "new.tar" || status.ErrorStage != tc.stage {
				t.Fatalf("unexpected status: %+v", status)
			}
			if status.LastSuccessfulUpdate == nil {
				t.Fatal("lost last successful update")
			}
			if tc.result == "applied" {
				if status.RunningImage == nil || status.RunningImage.Digest != "sha256:new" || status.RunningImage.Version != "v2" ||
					status.LastSuccessfulUpdate.Time != status.Time || status.LastSuccessfulUpdate.updateImage != *status.RunningImage {
					t.Fatalf("success must describe the running image: %+v", status)
				}
			} else if status.LastSuccessfulUpdate.Digest != "sha256:old" || status.LastSuccessfulUpdate.Version != "v1" ||
				status.LastSuccessfulUpdate.Time != "2026-01-01T00:00:00Z" {
				t.Fatalf("unsuccessful attempt replaced the last success: %+v", status)
			}
			if tc.result == "failed" && status.Error == "" {
				t.Error("failure must include a reason")
			}
			if tc.result != "failed" && status.Error != "" {
				t.Error("non-failed outcome retained an error")
			}
			if _, err := os.Stat(filepath.Join(f.updates, "new.tar")); !os.IsNotExist(err) {
				t.Fatal("processed archive was not removed")
			}
			log := f.read(filepath.Join(f.root, "commands"))
			if strings.Contains(log, "podman image prune") != (tc.result == "applied") {
				t.Fatalf("prune must only follow a confirmed success:\n%s", log)
			}
			if tc.name == "load failure" && strings.Contains(log, "podman auto-update") {
				t.Fatal("failed load triggered a restart")
			}
			if tc.result == "applied" && strings.Index(log, "skopeo copy") > strings.Index(log, "podman auto-update") {
				t.Fatal("restart happened before loading")
			}
		})
	}
}

func TestAgentUpdateStatusPublicationFailure(t *testing.T) {
	for _, env := range []string{"PUBLISH_FAIL=1", "RENAME_FAIL=1"} {
		t.Run(env, func(t *testing.T) {
			f := newUpdateFixture(t)
			statusPath := filepath.Join(f.updates, "status.json")
			f.write(statusPath, previousStatus, 0o644)
			archive := filepath.Join(f.updates, "new.tar")
			f.write(archive, "archive", 0o644)
			f.run("update", true, env)
			if f.read(statusPath) != previousStatus {
				t.Fatal("publication failure replaced the existing status")
			}
			if _, err := os.Stat(archive); err != nil {
				t.Fatal("publication failure must retain the archive for retry:", err)
			}
			if strings.Contains(f.read(filepath.Join(f.root, "commands")), "podman image prune") {
				t.Fatal("publication failure pruned the previous image")
			}
			leftovers, err := filepath.Glob(filepath.Join(f.updates, ".status.*"))
			if err != nil || len(leftovers) != 0 {
				t.Fatalf("temporary status files remain: %v, %v", leftovers, err)
			}
			// The image already started before publication failed. Retrying with that
			// same image is unchanged, not a newly confirmed successful update.
			f.run("update", false, `UPDATE_REPORT=[{"Unit":"planner-agent.service","Updated":"false"}]`)
			status := f.status()
			if status.Result != "unchanged" || status.Error != "" || status.ErrorStage != "" ||
				status.RunningImage == nil || status.RunningImage.Digest != "sha256:new" || status.RunningImage.Version != "v2" {
				t.Fatalf("retry must describe the actual running image: %+v", status)
			}
			if status.LastSuccessfulUpdate == nil || status.LastSuccessfulUpdate.Digest != "sha256:old" ||
				status.LastSuccessfulUpdate.Version != "v1" || status.LastSuccessfulUpdate.Time != "2026-01-01T00:00:00Z" {
				t.Fatalf("retry invented a successful-update record: %+v", status)
			}
			if _, err := os.Stat(archive); !os.IsNotExist(err) {
				t.Fatal("retry did not remove the processed archive")
			}
			if strings.Contains(f.read(filepath.Join(f.root, "commands")), "podman image prune") {
				t.Fatal("unchanged retry pruned the previous image")
			}
		})
	}
}

func TestAgentUpdatePersistenceAndStaging(t *testing.T) {
	f := newUpdateFixture(t)
	f.write(filepath.Join(f.updates, ".staging", "download.part"), "partial", 0o644)
	f.run("update", false)
	if _, err := os.Stat(filepath.Join(f.root, "commands")); !os.IsNotExist(err) {
		t.Fatal("staging file triggered image processing")
	}
	// Include control characters to verify that archive names are correctly JSON encoded.
	name := "new-\"\\\n.tar"
	f.write(filepath.Join(f.updates, name), "archive", 0o644)
	f.run("update", false)
	if status := f.status(); status.File != name || status.LastSuccessfulUpdate == nil {
		t.Fatalf("unexpected initial status: %+v", status)
	}
	before := f.read(filepath.Join(f.updates, "status.json"))
	// Simulate the boot loader with persistent image storage already populated. It must
	// skip the ISO copy and leave both the installed image and successful status intact.
	f.write(filepath.Join(f.root, "iso.tar"), "original image", 0o644)
	f.run("load", false)
	f.run("update", false)
	if after := f.read(filepath.Join(f.updates, "status.json")); after != before {
		t.Fatal("boot/repeated invocation changed the successful update record")
	}
	if log := f.read(filepath.Join(f.root, "commands")); strings.Count(log, "skopeo copy") != 1 {
		t.Fatalf("boot reloaded the original image:\n%s", log)
	}
	f.write(filepath.Join(f.updates, "broken.tar"), "bad archive", 0o644)
	f.run("update", false, "LOAD_EXIT=1")
	status := f.status()
	if status.Result != "failed" || status.LastSuccessfulUpdate == nil || status.LastSuccessfulUpdate.Version != "v2" {
		t.Fatalf("later failure lost the successful update: %+v", status)
	}
}

func TestAgentUpdateWithoutPreviousSuccess(t *testing.T) {
	for _, result := range []string{"false", "failed", "rolled back"} {
		t.Run(result, func(t *testing.T) {
			f := newUpdateFixture(t)
			f.write(filepath.Join(f.updates, "new.tar"), "archive", 0o644)
			f.run("update", false, fmt.Sprintf(`UPDATE_REPORT=[{"Unit":"planner-agent.service","Updated":%q}]`, result))
			if f.status().LastSuccessfulUpdate != nil {
				t.Fatal("non-applied attempt invented a successful update")
			}
		})
	}
}

func TestAgentUpdateFailureDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name, stage, message string
		env                  []string
	}{
		{name: "archive load", stage: "load", message: "no space left on device", env: []string{"LOAD_EXIT=1", "LOAD_STDERR=no space left on device"}},
		{name: "container restart", stage: "restart", message: "startup timed out", env: []string{"UPDATE_EXIT=1", "UPDATE_STDERR=startup timed out"}},
		{name: "restart error survives metadata failure", stage: "restart", message: "startup timed out", env: []string{
			"UPDATE_EXIT=1", "UPDATE_STDERR=startup timed out", "INSPECT_EXIT=1", "INSPECT_STDERR=container gone",
		}},
		{name: "report parsing", stage: "report", message: "Expected one planner-agent report", env: []string{`UPDATE_REPORT=[]`}},
		{name: "image inspection", stage: "metadata", message: "container inspect failed", env: []string{"INSPECT_EXIT=1", "INSPECT_STDERR=container inspect failed"}},
		{name: "bounded and JSON encoded", stage: "load", message: "tail \"quoted\"\nnext line", env: []string{
			"LOAD_EXIT=1", "LOAD_STDERR=discard-this-prefix" + strings.Repeat("x", 8192) + "tail \"quoted\"\nnext line",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newUpdateFixture(t)
			f.write(filepath.Join(f.updates, "new.tar"), "archive", 0o644)
			f.run("update", false, tc.env...)
			status := f.status()
			if status.Result != "failed" || status.ErrorStage != tc.stage || !strings.Contains(status.Error, tc.message) {
				t.Fatalf("missing stage or diagnostics: %+v", status)
			}
			if len(status.Error) > 4096+128 || strings.Contains(status.Error, "discard-this-prefix") {
				t.Fatal("diagnostics exceeded their bound")
			}
			leftovers, err := filepath.Glob(filepath.Join(f.updates, ".diagnostics.*"))
			if err != nil || len(leftovers) != 0 {
				t.Fatalf("temporary diagnostics remain: %v, %v", leftovers, err)
			}
		})
	}
}
