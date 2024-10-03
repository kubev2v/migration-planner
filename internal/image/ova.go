package image

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/coreos/butane/config"
	"github.com/coreos/butane/config/common"
	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/util"
	"github.com/openshift/assisted-image-service/pkg/isoeditor"
)

type Key int

// Key to store the ResponseWriter in the context of openapi
const ResponseWriterKey Key = 0

type Ova struct {
	Id     uuid.UUID
	Writer io.Writer
}

type Image interface {
	Generate() (io.Reader, error)
}

func (o *Ova) Generate() error {
	tw := tar.NewWriter(o.Writer)

	// Write ISO to TAR
	if err := o.writeIso(tw); err != nil {
		return err
	}

	// Write OVF to TAR
	if err := writeOvf(tw); err != nil {
		return err
	}

	return nil
}

func (o *Ova) writeIso(tw *tar.Writer) error {
	// Generate iginition
	ignitionContent, err := o.generateIgnition()
	if err != nil {
		return fmt.Errorf("error generating ignition: %w", err)
	}
	// Generate ISO data reader with ignition content
	reader, err := isoeditor.NewRHCOSStreamReader("rhcos-live.x86_64.iso", &isoeditor.IgnitionContent{Config: []byte(ignitionContent)}, nil, nil)
	if err != nil {
		return fmt.Errorf("error reading rhcos iso: %w", err)
	}
	// Create a header for AgentVM-1.iso
	length, err := reader.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	header := &tar.Header{
		Name:    "AgentVM-1.iso",
		Size:    length,
		Mode:    0600,
		ModTime: time.Now(),
	}

	// Write the header to the tar archive
	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	// Reset the reader to start
	_, err = reader.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}
	// Write ISO data to writer output
	if _, err = io.Copy(tw, reader); err != nil {
		return err
	}

	return nil
}

func writeOvf(tw *tar.Writer) error {
	ovfContent, err := os.ReadFile("data/AgentVM.ovf")
	if err != nil {
		return err
	}
	// Create a header for AgentVM.ovf
	header := &tar.Header{
		Name:    "AgentVM.ovf",
		Size:    int64(len(ovfContent)),
		Mode:    0600,
		ModTime: time.Now(),
	}

	// Write the header to the tar archive
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	if _, err := tw.Write([]byte(ovfContent)); err != nil {
		return err
	}

	return nil
}

func (o *Ova) generateIgnition() (string, error) {
	butaneTemplate, err := os.ReadFile("data/config.ign.template")
	if err != nil {
		return "", fmt.Errorf("error reading ignition template file: %w", err)
	}

	butaneContent := strings.Replace(string(butaneTemplate), "@CONFIG_ID@", o.Id.String(), -1)
	butaneContent = strings.Replace(butaneContent, "@CONFIG_SERVER@", util.GetEnv("CONFIG_SERVER", "http://127.0.0.1:7443"), -1)

	dataOut, _, err := config.TranslateBytes([]byte(butaneContent), common.TranslateBytesOptions{})
	if err != nil {
		return "", fmt.Errorf("error translating config: %w", err)
	}

	return string(dataOut), nil
}
