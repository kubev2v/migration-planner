package applianceupdater

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

type boundedOutput struct {
	bytes.Buffer
	Limit int
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	length := len(p)
	if remaining := b.Limit - b.Len(); remaining > 0 {
		_, _ = b.Buffer.Write(p[:min(remaining, len(p))])
	}
	return length, nil
}

func (h *Host) command(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(h.Config.CommandSeconds)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	stdout := &boundedOutput{Limit: 1024 * 1024}
	stderr := &boundedOutput{Limit: 4096}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	cmd.WaitDelay = 5 * time.Second
	if err := cmd.Run(); err != nil {
		// Agent/Podman output can contain customer data. Keep the public failure
		// diagnostic independent of command stdout, stderr, and arguments.
		return nil, fmt.Errorf("%s command failed: %w", name, err)
	}
	if stdout.Len() >= stdout.Limit {
		return nil, fmt.Errorf("%s output exceeds limit", name)
	}
	return stdout.Bytes(), nil
}
