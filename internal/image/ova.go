package image

import (
	"archive/tar"
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"io"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/coreos/butane/config"
	"github.com/coreos/butane/config/common"
	"github.com/golang-jwt/jwt/v5"
	"github.com/kubev2v/migration-planner/internal/util"
	"github.com/openshift/assisted-image-service/pkg/isoeditor"
	"github.com/openshift/assisted-image-service/pkg/overlay"
	"go.uber.org/zap"
)

type Key int

// Key to store the ResponseWriter in the context of openapi
const ResponseWriterKey Key = 0

// CertificateProvider provides the entire certificate chain and private key.
// The provider (Vault, selfsigned ...) must implement this interface.
type CertificateProvider interface {
	GetCACertificate(expire time.Time) (*x509.Certificate, *rsa.PrivateKey, error)
	GetCertificate(caCert *x509.Certificate, caKey *rsa.PrivateKey) (*x509.Certificate, *rsa.PrivateKey, error)
	ConvertToPEM(cert *x509.Certificate, key *rsa.PrivateKey) ([]byte, []byte)
}

type Ova struct {
	Writer       io.Writer
	SshKey       *string
	Jwt          *jwt.Token
	CertProvider CertificateProvider
}

// IgnitionData defines modifiable fields in ignition config
type IgnitionData struct {
	SshKey                     string
	PlannerServiceUI           string
	PlannerService             string
	MigrationPlannerAgentImage string
	InsecureRegistry           string
	Token                      string
	UICaCertificate            string
	UICertificate              string
	UIPrivateKey               string
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

func calculateTarSize(contentSize int) int {
	const blockSize = 512

	// Size of the tar header block
	size := blockSize

	// Size of the file content, rounded up to nearest 512 bytes
	size += ((contentSize + blockSize - 1) / blockSize) * blockSize

	return size
}

func (o *Ova) OvaSize() (int, error) {
	isoSize, err := o.isoSize()
	if err != nil {
		return -1, err
	}
	ovfSize, err := o.ovfSize()
	if err != nil {
		return -1, err
	}

	return ovfSize + isoSize, nil
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

	tw.Flush()

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
	// Create a header for MigrationAssessment.iso
	length, err := reader.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	header := &tar.Header{
		Name:    "MigrationAssessment.iso",
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

func (o *Ova) isoSize() (int, error) {
	// Get ISO reader
	reader, err := o.isoReader()
	if err != nil {
		return -1, err
	}
	length, err := reader.Seek(0, io.SeekEnd)
	if err != nil {
		return -1, err
	}

	// Reset the reader to start
	_, err = reader.Seek(0, io.SeekStart)
	if err != nil {
		return -1, err
	}

	return calculateTarSize(int(length)), nil
}

func (o *Ova) ovfSize() (int, error) {
	file, err := os.Open("data/MigrationAssessment.ovf")
	if err != nil {
		return -1, err
	}
	defer file.Close()
	fileInfo, err := file.Stat()
	if err != nil {
		return -1, err
	}

	return calculateTarSize(int(fileInfo.Size())), nil
}

func writeOvf(tw *tar.Writer) error {
	ovfContent, err := os.ReadFile("data/MigrationAssessment.ovf")
	if err != nil {
		return err
	}
	// Create a header for AgentVM.ovf
	header := &tar.Header{
		Name:    "MigrationAssessment.ovf",
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

	// generate ui certificates if cert provider is provided
	if o.CertProvider != nil {
		caCertificate, caPrivateKey, err := o.CertProvider.GetCACertificate(time.Now().AddDate(1, 0, 0)) // expire in 1 year
		if err != nil {
			return "", err
		}

		uiCertificate, uiPrivateKey, err := o.CertProvider.GetCertificate(caCertificate, caPrivateKey)
		if err != nil {
			return "", err
		}

		uiCertPem, uiPrivateKeyPem := o.CertProvider.ConvertToPEM(uiCertificate, uiPrivateKey)
		caCertPem, _ := o.CertProvider.ConvertToPEM(caCertificate, uiPrivateKey)

		ignData.UICaCertificate = string(caCertPem)
		ignData.UICertificate = string(uiCertPem)
		ignData.UIPrivateKey = string(uiPrivateKeyPem)

		zap.S().Named("ova").Debug("UI certificates generated")
	}

	var buf bytes.Buffer
	t, err := template.New("ignition.template").Funcs(getTemplateFuncMap()).ParseFiles("data/ignition.template")
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

func getTemplateFuncMap() template.FuncMap {
	writeCertificateFunc := func(cert string, intend int) string {
		var sb strings.Builder
		spacer := ""
		for {
			if intend == 0 {
				break
			}
			spacer = fmt.Sprintf(" %s", spacer)
			intend -= 1
		}
		lines := strings.Split(cert, "\n")
		for idx, l := range lines {
			if l == "" {
				continue
			}
			if idx < len(lines)-1 {
				fmt.Fprintf(&sb, "%s%s\n", spacer, strings.TrimSpace(l))
				continue
			}
			fmt.Fprintf(&sb, "%s%s", spacer, strings.TrimSpace(l))
		}
		return sb.String()
	}

	return template.FuncMap{
		"write_certificate": writeCertificateFunc,
	}
}
