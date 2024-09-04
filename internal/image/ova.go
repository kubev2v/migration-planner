package image

import (
	"archive/tar"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/kubev2v/migration-planner/internal/util"
	"github.com/openshift/assisted-image-service/pkg/isoeditor"
)

type Ova struct {
	Id uint64
}

type Image interface {
	Generate() (io.Reader, error)
}

func (o *Ova) Generate() (io.Reader, error) {
	// Generate iginition
	ignitionContent, err := o.generateIgnition()
	if err != nil {
		return nil, fmt.Errorf("error generating ignition: %w", err)
	}

	// Genreate TAR file
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	reader, err := isoeditor.NewRHCOSStreamReader("rhcos-live.x86_64.iso", &isoeditor.IgnitionContent{Config: []byte(ignitionContent)}, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("error reading ISO file: %w", err)
	}
	isoData, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("error reading ISO file: %w", err)
	}
	// Create a header for AgentVM-1.iso
	header := &tar.Header{
		Name:    "AgentVM-1.iso",
		Size:    int64(len(isoData)),
		Mode:    0600,
		ModTime: time.Now(),
	}

	// Write the header to the tar archive
	if err := tw.WriteHeader(header); err != nil {
		return nil, err
	}

	if _, err := tw.Write([]byte(isoData)); err != nil {
		return nil, err
	}

	// Write OVF file
	ovfContent, err := os.ReadFile("data/AgentVM.ovf")
	if err != nil {
		return nil, fmt.Errorf("error reading OVF file: %w", err)
	}
	// Create a header for AgentVM.ovf
	header = &tar.Header{
		Name:    "AgentVM.ovf",
		Size:    int64(len(ovfContent)),
		Mode:    0600,
		ModTime: time.Now(),
	}

	// Write the header to the tar archive
	if err := tw.WriteHeader(header); err != nil {
		return nil, err
	}
	if _, err := tw.Write([]byte(ovfContent)); err != nil {
		return nil, err
	}

	// Close the tar writer
	if err := tw.Close(); err != nil {
		return nil, err
	}

	return &buf, nil
}

func (o *Ova) generateIgnition() (string, error) {
	ignitionContent := ""
	ip := util.GetEnv("CONFIG_IP", "127.0.0.1")

	cfgTemplate, err := os.ReadFile("data/config.yaml.template")
	if err != nil {
		return ignitionContent, fmt.Errorf("error reading OVF template file: %w", err)
	}
	cfgContent := strings.Replace(string(cfgTemplate), "@CONFIG_ID@", fmt.Sprintf("%d", o.Id), -1)
	cfgContent = strings.Replace(string(cfgContent), "@CONFIG_IP@", ip, -1)

	// gen config.ign
	ignTemplate, err := os.ReadFile("data/config.ign.template")
	if err != nil {
		return ignitionContent, fmt.Errorf("error reading OVF template file: %w", err)
	}
	ignitionContent = strings.Replace(string(ignTemplate), "@CONFIG_DATA@", base64.StdEncoding.EncodeToString([]byte(cfgContent)), -1)

	return ignitionContent, nil
}
