package applianceupdater

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	spec "github.com/opencontainers/runtime-spec/specs-go"
	"go.podman.io/image/v5/docker/reference"
	"go.podman.io/podman/v6/pkg/bindings/containers"
	"go.podman.io/podman/v6/pkg/bindings/images"
	"go.podman.io/podman/v6/pkg/specgen"
)

// Host controls systemd and runs the pinned agent CLI through the Podman API.
type Host struct {
	Config       Config
	podmanSocket string
}

func NewHost(c Config) (*Host, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &Host{Config: c}, nil
}

func (h *Host) Inspect(ctx context.Context, ref string) (Image, error) {
	ctx, close, err := h.connectPodman(ctx)
	if err != nil {
		return Image{}, err
	}
	defer close()
	record, err := images.GetImage(ctx, ref, nil)
	if err != nil {
		return Image{}, fmt.Errorf("inspect image: %w", err)
	}
	id := record.ID
	if len(id) == 64 {
		id = "sha256:" + id
	}
	if !imageID.MatchString(id) {
		return Image{}, fmt.Errorf("image inspection requires an immutable ID")
	}
	return Image{id, string(record.Digest), record.Labels["org.opencontainers.image.version"]}, nil
}

func (h *Host) Running(ctx context.Context) (Image, error) {
	api, close, err := h.connectPodman(ctx)
	if err != nil {
		return Image{}, err
	}
	defer close()
	container, err := containers.Inspect(api, "planner-agent", nil)
	if err != nil {
		return Image{}, fmt.Errorf("inspect agent: %w", err)
	}
	if container.State == nil || !container.State.Running || container.State.Health == nil || container.State.Health.Status != "healthy" {
		return Image{}, fmt.Errorf("agent container is not running and healthy")
	}
	return h.Inspect(ctx, container.Image)
}

func (h *Host) Load(ctx context.Context, archive, tag string) (Image, error) {
	// Archive extraction can exceed the live host's temporary filesystem.
	// Keep it on the same persistent disk as the uploaded image.
	loading := filepath.Join(h.Config.DataDir, "updates/.loading")
	if err := os.MkdirAll(loading, 0700); err != nil {
		return Image{}, err
	}
	tmp, err := os.MkdirTemp(loading, "archive-")
	if err != nil {
		return Image{}, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if _, err := h.command(ctx, "skopeo", "copy", "--tmpdir", tmp, "oci-archive:"+archive, "containers-storage:"+tag); err != nil {
		return Image{}, err
	}
	return h.Inspect(ctx, tag)
}

func (h *Host) Pin(ctx context.Context, image Image, tag string) error {
	named, err := reference.ParseNormalizedNamed(tag)
	if err != nil {
		return err
	}
	tagged, ok := reference.TagNameOnly(named).(reference.NamedTagged)
	if !ok {
		return fmt.Errorf("image selection requires a tag")
	}
	ctx, close, err := h.connectPodman(ctx)
	if err != nil {
		return err
	}
	defer close()
	return images.Tag(ctx, image.ID, tagged.Tag(), reference.TrimNamed(named).String(), nil)
}

func (h *Host) Select(ctx context.Context, image Image) error {
	return h.Pin(ctx, image, h.Config.ImageTag)
}

func (h *Host) Stop(ctx context.Context) error {
	if _, err := h.command(ctx, "systemctl", "--user", "stop", "planner-agent.service"); err != nil {
		return err
	}
	return h.VerifyStopped(ctx)
}

// VerifyStopped checks every rootless container, including bind mounts covering
// an ancestor or descendant of the data directory. Other consumers are rejected,
// never killed automatically. All supported application starts must use Gate.
func (h *Host) VerifyStopped(ctx context.Context) error {
	dataDir, err := filepath.EvalSymlinks(h.Config.DataDir)
	if err != nil {
		return err
	}
	ctx, close, err := h.connectPodman(ctx)
	if err != nil {
		return err
	}
	defer close()
	inventory, err := containers.List(ctx, new(containers.ListOptions).WithAll(true))
	if err != nil {
		return fmt.Errorf("list data consumers: %w", err)
	}
	for _, item := range inventory {
		if !imageID.MatchString("sha256:" + item.ID) {
			return fmt.Errorf("invalid container inventory")
		}
		record, err := containers.Inspect(ctx, item.ID, nil)
		if err != nil {
			return fmt.Errorf("inspect data consumer: %w", err)
		}
		if record.State == nil || record.State.Status == "" {
			return fmt.Errorf("container process state is missing")
		}
		stopped := !record.State.Running && !record.State.Paused && !record.State.Restarting && record.State.Pid == 0 && slices.Contains([]string{"configured", "created", "exited", "stopped"}, record.State.Status)
		if stopped {
			continue
		}
		if strings.TrimPrefix(record.Name, "/") == "planner-agent" {
			return fmt.Errorf("agent container processes have not exited")
		}
		for _, mount := range record.Mounts {
			if mount.Type != "bind" && mount.Type != "volume" {
				continue
			}
			source, err := filepath.EvalSymlinks(mount.Source)
			if err != nil {
				return fmt.Errorf("cannot verify a live container mount")
			}
			fromData, dataErr := filepath.Rel(dataDir, source)
			fromMount, mountErr := filepath.Rel(source, dataDir)
			insideData := fromData != ".." && !strings.HasPrefix(fromData, ".."+string(os.PathSeparator))
			coversData := fromMount != ".." && !strings.HasPrefix(fromMount, ".."+string(os.PathSeparator))
			if dataErr != nil || mountErr != nil || insideData || coversData {
				return fmt.Errorf("another container may still access the agent data directory")
			}
		}
	}
	return nil
}

func (h *Host) Start(ctx context.Context) error {
	if _, err := h.command(ctx, "systemctl", "--user", "reset-failed", "planner-agent.service"); err != nil {
		return err
	}
	_, err := h.command(ctx, "systemctl", "--user", "start", "planner-agent.service")
	return err
}

func (h *Host) Suspend(ctx context.Context) error {
	_, err := h.command(ctx, "systemctl", "--user", "stop", "planner-agent-image-update.path")
	return err
}

func (h *Host) Migrate(ctx context.Context, attempt Journal, operation string) error {
	name := "oma-upgrade-" + attempt.AttemptID
	// A killed coordinator can leave its migration container alive.
	// Stop that exact attempt's previous CLI before any new offline operation.
	if err := h.CancelOffline(ctx, attempt); err != nil {
		return err
	}
	if err := h.VerifyStopped(ctx); err != nil {
		return err
	}
	container := specgen.NewSpecGenerator(attempt.Candidate.ID, false)
	container.Name = name
	container.Entrypoint = []string{"/app/agent"}
	container.UserNS = specgen.Namespace{NSMode: specgen.KeepID, Value: "uid=1001"}
	container.Mounts = []spec.Mount{{Type: "bind", Source: h.Config.DataDir, Destination: "/var/lib/agent", Options: []string{"rw", "rbind", "Z"}}}
	container.Command = []string{"migrate", operation, "--data-folder", "/var/lib/agent", "--attempt-id", attempt.AttemptID}
	switch operation {
	case "prepare":
		container.Command = append(container.Command, "--previous-image", attempt.Previous.ID, "--candidate-image", attempt.Candidate.ID)
	case "verify":
		container.Command = append(container.Command, "--target", "baseline")
	}
	data, err := h.runContainer(ctx, container)
	if err != nil {
		return err
	}
	var result migrationResult
	if err := strictDecode(data, &result); err != nil {
		return fmt.Errorf("invalid offline migration result")
	}
	matchesRequest := result.AttemptID == attempt.AttemptID && result.Operation == operation
	succeeded := !result.DryRun && result.Error == "" && len(result.Databases) > 0
	if !matchesRequest || !succeeded {
		return fmt.Errorf("invalid offline migration result")
	}
	return nil
}

func (h *Host) CancelOffline(ctx context.Context, attempt Journal) error {
	return h.removeContainer(ctx, "oma-upgrade-"+attempt.AttemptID)
}

type migrationResult struct {
	AttemptID string          `json:"attempt_id"`
	Operation string          `json:"operation"`
	DryRun    bool            `json:"dry_run"`
	Databases json.RawMessage `json:"databases"`
	Error     string          `json:"error,omitempty"`
}
