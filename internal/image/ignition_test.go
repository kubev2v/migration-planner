package image

import (
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/kubev2v/migration-planner/internal/applianceupdater"
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

func TestIgnitionTemplateHostCoordinator(t *testing.T) {
	files, links := ignitionFiles(t)
	quadlet := files["/home/core/.config/containers/systemd/planner-agent.container"]
	for _, want := range []string{
		"HealthCmd=curl -fsk https://localhost:3333/api/v2/version",
		"Notify=healthy", "TimeoutStartSec=660", "Pull=never",
		"Requires=podman.socket", "After=podman.socket",
		"Volume=/var/lib/data/agent:/var/lib/agent:Z",
		"EnvironmentFile=/home/core/.migration-planner/upgrade.env",
		"ExecStartPre=/usr/local/sbin/appliance-updater gate",
	} {
		if !strings.Contains(quadlet, want) {
			t.Errorf("Quadlet missing %q", want)
		}
	}
	if strings.Contains(quadlet, "autoupdate") || strings.Contains(quadlet, "After=planner-agent-image-recovery") {
		t.Fatal("agent must not bypass coordination or wait on a recovery service that starts it")
	}
	if files["/usr/local/sbin/update-planner-agent-image.sh"] != "" {
		t.Fatal("legacy auto-update script remains installed")
	}
	for _, tc := range []struct{ file, want string }{
		{"planner-agent-image-update.path", "PathExistsGlob=/var/lib/data/agent/updates/*.tar"},
		{"planner-agent-image-update.path", "After=planner-agent-image-recovery.service planner-agent.service"},
		{"planner-agent-image-update.service", "Requires=podman.socket"},
		{"planner-agent-image-recovery.service", "After=podman.socket"},
		{"planner-agent-image-update.service", "ExecStart=/usr/local/sbin/appliance-updater update"},
		{"planner-agent-image-recovery.service", "ExecStart=/usr/local/sbin/appliance-updater recover"},
	} {
		if !strings.Contains(files["/home/core/.config/systemd/user/"+tc.file], tc.want) {
			t.Errorf("%s missing %s", tc.file, tc.want)
		}
	}
	var cfg applianceupdater.Config
	configJSON := files["/home/core/.migration-planner/upgrade.json"]
	decoder := json.NewDecoder(strings.NewReader(configJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Transitions) != 0 || cfg.ImageTag != defaultAgentImage {
		t.Fatal("untested transitions enabled")
	}
	var recoveryEnabled, socketEnabled bool
	for _, link := range links {
		if strings.Contains(link, "podman-auto-update.timer") {
			t.Fatal("competing auto-update timer enabled")
		}
		socketEnabled = socketEnabled || strings.HasSuffix(link, "sockets.target.wants/podman.socket")
		recoveryEnabled = recoveryEnabled || strings.HasSuffix(link, "default.target.wants/planner-agent-image-recovery.service")
	}
	if !socketEnabled {
		t.Fatal("rootless Podman API socket not enabled")
	}
	if !recoveryEnabled {
		t.Fatal("boot recovery not enabled")
	}
	template, err := os.ReadFile("../../data/ignition.template")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(template), "/usr/bin/install -m 0755 /run/media/iso/host/appliance-updater") {
		t.Fatal("host executable not installed independently of agent container")
	}
}

func TestBootLoaderPreservesPersistentImage(t *testing.T) {
	files, _ := ignitionFiles(t)
	folder := t.TempDir()
	script := strings.ReplaceAll(files["/usr/local/sbin/load-planner-agent-image.sh"], "/run/media/iso/images/migration-planner-agent.tar", filepath.Join(folder, "original.tar"))
	for name, content := range map[string]string{
		"original.tar": "original image",
		"load.sh":      script,
		"podman":       "#!/bin/sh\n[ \"$1 $2\" = \"image exists\" ]\n",
		"skopeo":       "#!/bin/sh\nexit 99\n",
	} {
		if err := os.WriteFile(filepath.Join(folder, name), []byte(content), 0755); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("bash", filepath.Join(folder, "load.sh"))
	cmd.Env = append(os.Environ(), "PATH="+folder+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("boot reloaded the ISO image: %v %s", err, out)
	}
}
