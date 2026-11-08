package applianceupdater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go.podman.io/podman/v6/pkg/specgen"
)

// CheckExtensions permits only transitions that reuse the installed extension
// bundle byte for byte. Engine/extension changes need a separate provisioning and
// recovery contract; the host never silently replaces assets.
func (h *Host) CheckExtensions(ctx context.Context, image Image) error {
	container := specgen.NewSpecGenerator(image.ID, false)
	container.Entrypoint = []string{"/bin/sh"}
	container.Command = []string{"-c", "sha256sum /app/extensions/*.duckdb_extension"}
	readOnly := true
	container.ReadOnlyFilesystem = &readOnly
	data, err := h.runContainer(ctx, container)
	if err != nil {
		return fmt.Errorf("cannot inspect bundled DuckDB extensions: %w", err)
	}
	root, err := os.OpenRoot(h.Config.DataDir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	seen := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || !imageID.MatchString("sha256:"+fields[0]) || !strings.HasPrefix(fields[1], "/app/extensions/") {
			return fmt.Errorf("invalid bundled extension checksums")
		}
		name := strings.TrimPrefix(fields[1], "/app/extensions/")
		if filepath.Base(name) != name || !strings.HasSuffix(name, ".duckdb_extension") || seen[name] {
			return fmt.Errorf("invalid bundled extension name")
		}
		seen[name] = true
		file, err := root.Open(name)
		if err != nil {
			return fmt.Errorf("required installed extension is unavailable")
		}
		info, statErr := file.Stat()
		if statErr != nil || !info.Mode().IsRegular() || info.Size() > 256*1024*1024 {
			_ = file.Close()
			return fmt.Errorf("invalid installed extension file")
		}
		hash := sha256.New()
		_, readErr := io.Copy(hash, io.LimitReader(file, 256*1024*1024+1))
		if err := errors.Join(readErr, file.Close()); err != nil {
			return err
		}
		if hex.EncodeToString(hash.Sum(nil)) != fields[0] {
			return fmt.Errorf("DuckDB extension changes require a tested provisioning and recovery contract")
		}
	}
	if !seen["sqlite_scanner.duckdb_extension"] {
		return fmt.Errorf("image lacks the required sqlite extension bundle")
	}
	return nil
}
