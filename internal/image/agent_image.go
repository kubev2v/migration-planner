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
// deployment replaces both; http.ServeFile provides Range and conditional
// request support.
func ServeAgentImageTar(w http.ResponseWriter, r *http.Request, isoPath string) error {
	path := AgentImageTarPath(isoPath)
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	w.Header().Set("ETag", fmt.Sprintf(`"%x-%x"`, info.Size(), info.ModTime().UnixNano()))
	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, AgentImageTarName))
	http.ServeFile(w, r, path)
	return nil
}
