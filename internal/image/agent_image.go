package image

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

// AgentImageTarName is the appliance image OCI archive embedded in the served
// ISO and extracted next to it by the pull-iso init container.
const AgentImageTarName = "migration-planner-agent.tar"

// AgentImageTarPath returns the appliance image tar served alongside the ISO.
func AgentImageTarPath(isoPath string) string {
	return filepath.Join(filepath.Dir(isoPath), AgentImageTarName)
}

// ServeAgentImageTar serves the extracted appliance image tar. The ETag is
// size and mtime of the extracted file, so it changes exactly when a new ISO
// deployment replaces both; http.ServeContent provides Range and conditional
// request support. The file is opened once so the ETag and the body always
// describe the same file even if a deployment replaces it concurrently.
func ServeAgentImageTar(w http.ResponseWriter, r *http.Request, isoPath string) error {
	path := AgentImageTarPath(isoPath)
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	w.Header().Set("ETag", fmt.Sprintf(`"%x-%x"`, info.Size(), info.ModTime().UnixNano()))
	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, AgentImageTarName))
	http.ServeContent(w, r, AgentImageTarName, info.ModTime(), f)
	return nil
}
