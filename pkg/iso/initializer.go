package iso

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/google/uuid"

	"go.uber.org/zap"
)

type IsoDownloader interface {
	Download(context.Context, io.WriteSeeker) error
}

type IsoInitializer struct {
	downloader IsoDownloader
}

func NewIsoInitializer(downloader IsoDownloader) *IsoInitializer {
	return &IsoInitializer{downloader: downloader}
}

// Initialize ensures the target ISO file is available and valid by checking its SHA256 hash.
// If the file exists and its SHA256 matches the expected value, no action is taken.
// If the file is missing or has an incorrect hash, it downloads the new image to a temporary file,
// then atomically replaces the existing file using intermediate naming to prevent corruption.
// The method includes rollback logic and cleanup of temporary files to maintain system integrity.
func (i *IsoInitializer) Initialize(ctx context.Context, targetIsoFile string, targetIsoSha256 string) error {
	statErr := i.verifyIso(targetIsoFile, targetIsoSha256)
	if statErr == nil {
		return nil
	}

	zap.S().Debugw("failed to verify the integrity of the existing image", "error", statErr, "target iso", targetIsoFile, "target sha256", targetIsoSha256)

	// first try to download the new image to temporary file
	tempIsoFile, err := os.CreateTemp(path.Dir(targetIsoFile), "iso-image")
	if err != nil {
		return fmt.Errorf("failed to create temporary iso file: %w", err)
	}

	defer func() {
		_ = os.Remove(tempIsoFile.Name())
	}()

	if err := i.downloader.Download(ctx, tempIsoFile); err != nil {
		tempIsoFile.Close()
		return fmt.Errorf("failed to write the image to the temporary iso file: %w", err)
	}
	tempIsoFile.Close()

	// if targetIsoFile does not exists, just move the new the targetIsoFile
	if errors.Is(statErr, os.ErrNotExist) {
		return os.Rename(tempIsoFile.Name(), targetIsoFile)
	}

	intermediateFilename := i.createIntermediateFilename(targetIsoFile)

	if err := os.Rename(targetIsoFile, intermediateFilename); err != nil {
		return fmt.Errorf("failed to rename the old image iso: %w", err)
	}

	if err := os.Rename(tempIsoFile.Name(), targetIsoFile); err != nil {
		// try to roll back the old image
		rbErr := os.Rename(intermediateFilename, targetIsoFile)
		if rbErr != nil {
			return fmt.Errorf("failed to promote new iso to target: %v; rollback also failed: %w", err, rbErr)
		}
		return fmt.Errorf("failed to promote new iso to target: %w", err)
	}

	// safe to remove the old file
	if err := os.Remove(intermediateFilename); err != nil {
		zap.S().Warnw("failed to remove the old iso image from storage", "error", err)
	}

	return nil
}

func (i *IsoInitializer) verifyIso(targetIsoFile, targetIsoSha256 string) error {
	if _, err := os.Stat(targetIsoFile); err != nil {
		return err
	}

	// compute sha256
	reader, err := os.Open(targetIsoFile)
	if err != nil {
		return err
	}
	defer reader.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, reader); err != nil {
		return err
	}

	computedSha256 := fmt.Sprintf("%x", hasher.Sum(nil))

	if targetIsoSha256 != computedSha256 {
		return fmt.Errorf("sha256 sums dont't match. computed %s wanted %s", computedSha256, targetIsoSha256)
	}

	return nil
}

func (i *IsoInitializer) createIntermediateFilename(targetIsoFile string) string {
	baseName := strings.TrimSuffix(path.Base(targetIsoFile), path.Ext(targetIsoFile))
	ext := path.Ext(targetIsoFile)
	return path.Join(path.Dir(targetIsoFile), fmt.Sprintf("%s%x%s", baseName, uuid.NewString()[:6], ext))
}
