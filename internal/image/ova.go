package image

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"text/template"
	"time"

	"github.com/coreos/butane/config"
	"github.com/coreos/butane/config/common"
	"github.com/golang-jwt/jwt/v5"
	"github.com/kubev2v/migration-planner/internal/util"
	"github.com/openshift/assisted-image-service/pkg/isoeditor"
	"github.com/openshift/assisted-image-service/pkg/overlay"
)

type Key int

// Key to store the ResponseWriter in the context of openapi
const ResponseWriterKey Key = 0

type Ova struct {
	Writer io.Writer
	SshKey *string
	Jwt    *jwt.Token
}

// IgnitionData defines modifiable fields in ignition config
type IgnitionData struct {
	SshKey                     string
	PlannerServiceUI           string
	PlannerService             string
	MigrationPlannerAgentImage string
	InsecureRegistry           string
	Token                      string
}

type Image interface {
	Generate() (io.Reader, error)
	Validate() (io.Reader, error)
}

func (o *Ova) Validate() error {
	if _, err := o.isoReader(); err != nil {
		return err
	}

	return nil
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

func (o *Ova) isoReader() (overlay.OverlayReader, error) {
	// Generate iginition
	ignitionContent, err := o.generateIgnition()
	if err != nil {
		return nil, fmt.Errorf("error generating ignition: %w", err)
	}
	// Generate ISO data reader with ignition content
	reader, err := isoeditor.NewRHCOSStreamReader("rhcos-live.x86_64.iso", &isoeditor.IgnitionContent{Config: []byte(ignitionContent)}, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("error reading rhcos iso: %w", err)
	}

	return reader, nil
}

func (o *Ova) writeIso(tw *tar.Writer) error {
	// Get ISO reader
	reader, err := o.isoReader()
	if err != nil {
		return err
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
	ignData := IgnitionData{
		PlannerService:             util.GetEnv("CONFIG_SERVER", "http://127.0.0.1:7443"),
		PlannerServiceUI:           util.GetEnv("CONFIG_SERVER_UI", "http://localhost:3000/migrate/wizard"),
		MigrationPlannerAgentImage: util.GetEnv("MIGRATION_PLANNER_AGENT_IMAGE", "quay.io/kubev2v/migration-planner-agent"),
	}
	if o.SshKey != nil {
		ignData.SshKey = *o.SshKey
	}
	if o.Jwt != nil {
		ignData.Token = o.Jwt.Raw
	}

	if insecureRegistry := os.Getenv("INSECURE_REGISTRY"); insecureRegistry != "" {
		ignData.InsecureRegistry = insecureRegistry
	}

	var buf bytes.Buffer
	t, err := template.New("ignition.template").ParseFiles("data/ignition.template")
	if err != nil {
		return "", fmt.Errorf("error reading the ignition template: %w", err)
	}
	if err := t.Execute(&buf, ignData); err != nil {
		return "", fmt.Errorf("error parsing the ignition template: %w", err)
	}

	dataOut, _, err := config.TranslateBytes(buf.Bytes(), common.TranslateBytesOptions{})
	if err != nil {
		return "", fmt.Errorf("error translating config: %w", err)
	}

	return string(dataOut), nil
}
