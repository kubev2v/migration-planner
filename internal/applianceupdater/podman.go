package applianceupdater

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"go.podman.io/podman/v6/pkg/bindings"
	"go.podman.io/podman/v6/pkg/bindings/containers"
	"go.podman.io/podman/v6/pkg/specgen"
)

// Each operation has its own connection and deadline. Only the local core user's
// socket is used; environment variables cannot redirect upgrades to another host.
func (h *Host) connectPodman(ctx context.Context) (context.Context, func(), error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(h.Config.CommandSeconds)*time.Second)
	socket := h.podmanSocket
	if socket == "" {
		socket = fmt.Sprintf("unix:///run/user/%d/podman/podman.sock", os.Getuid())
	}
	api, err := bindings.NewConnection(ctx, socket)
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("connect to local Podman: %w", err)
	}
	client, err := bindings.GetClient(api)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	return api, func() { cancel(); client.Client.CloseIdleConnections() }, nil
}

func (h *Host) removeContainer(ctx context.Context, name string) error {
	ctx, close, err := h.connectPodman(ctx)
	if err != nil {
		return err
	}
	defer close()
	reports, err := containers.Remove(ctx, name, new(containers.RemoveOptions).WithForce(true).WithIgnore(true))
	if err != nil {
		return fmt.Errorf("remove offline container: %w", err)
	}
	for _, report := range reports {
		if report.Err != nil {
			return fmt.Errorf("remove offline container: %w", report.Err)
		}
	}
	return nil
}

// Keep the container until its exit code and output have been collected. Always
// remove it afterwards, including when the caller's deadline has expired.
func (h *Host) runContainer(ctx context.Context, spec *specgen.SpecGenerator) (output []byte, err error) {
	api, close, err := h.connectPodman(ctx)
	if err != nil {
		return nil, err
	}
	defer close()
	if spec.Name == "" {
		spec.Name = "oma-upgrade-probe-" + uuid.NewString()
	}
	spec.NetNS = specgen.Namespace{NSMode: specgen.NoNetwork}
	spec.LogConfiguration = &specgen.LogConfig{Driver: "k8s-file", Size: 2 * 1024 * 1024}
	created, err := containers.CreateWithSpec(api, spec, nil)
	if err != nil {
		// A lost response may still have created the named migration container.
		err = errors.Join(err, h.removeContainer(context.WithoutCancel(ctx), spec.Name))
		return nil, fmt.Errorf("create offline container: %w", err)
	}
	defer func() { err = errors.Join(err, h.removeContainer(context.WithoutCancel(ctx), created.ID)) }()
	if err := containers.Start(api, created.ID, nil); err != nil {
		return nil, fmt.Errorf("start offline container: %w", err)
	}
	code, err := containers.Wait(api, created.ID, nil)
	if err != nil {
		return nil, fmt.Errorf("wait for offline container: %w", err)
	}
	if code != 0 {
		return nil, fmt.Errorf("offline container exited with code %d", code)
	}
	return containerOutput(api, created.ID)
}

// Drain both streams, retaining only bounded stdout. Agent output can contain
// customer data, so stderr is never included in public errors.
func containerOutput(ctx context.Context, id string) ([]byte, error) {
	stdout, stderr := make(chan string), make(chan string)
	done := make(chan error, 1)
	go func() {
		done <- containers.Logs(ctx, id, new(containers.LogOptions).WithStdout(true).WithStderr(true), stdout, stderr)
	}()
	output := &boundedOutput{Limit: 1024 * 1024}
	for {
		select {
		case text := <-stdout:
			_, _ = output.Write([]byte(text))
		case <-stderr:
		case err := <-done:
			if err != nil {
				return nil, fmt.Errorf("read offline container output: %w", err)
			}
			if output.Len() >= output.Limit {
				return nil, fmt.Errorf("offline container output exceeds limit")
			}
			return output.Bytes(), nil
		}
	}
}
