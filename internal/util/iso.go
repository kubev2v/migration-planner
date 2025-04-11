package util

import (
	"fmt"
	"io"
	"net/http"
	"os"
)

const rhcosUrl string = "https://mirror.openshift.com/pub/openshift-v4/dependencies/rhcos/latest/rhcos-live.x86_64.iso"

// InitiliazeIso will check if the ISO on already exists at specified path
// and if not it will download it from the specified URL
func InitiliazeIso() error {

	// Check if ISO already exists:
	isoPath := GetEnv("MIGRATION_PLANNER_ISO_PATH", "rhcos-live.x86_64.iso")
	if _, err := os.Stat(isoPath); err == nil {
		return nil
	}

	// Download ISO
	isoURL := GetEnv("MIGRATION_PLANNER_ISO_URL", rhcosUrl)
	resp, err := http.Get(isoURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download ISO, status code: %d", resp.StatusCode)
	}

	out, err := os.Create(isoPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}
