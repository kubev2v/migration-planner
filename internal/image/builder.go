package image

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"text/template"
	"time"

	"github.com/coreos/butane/config"
	"github.com/coreos/butane/config/common"
	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/util"
	"github.com/openshift/assisted-image-service/pkg/isoeditor"
	"github.com/openshift/assisted-image-service/pkg/overlay"
)

type ImageType int

const (
	OVAImageType ImageType = iota
	QemuImageType
)

const (
	defaultAgentImage            = "quay.io/kubev2v/migration-planner-agent"
	defaultPlannerService        = "http://127.0.0.1:7443"
	defaultPersistenceDiskDevice = "/dev/sda"
	defaultConfigServerUI        = "http://localhost:3000/migrate/wizard"
	defaultTemplate              = "data/ignition.template"
	defaultPersistentDiskImage   = "data/persistence-disk.vmdk"
	defaultOvfFile               = "data/MigrationAssessment.ovf"
	defaultOvfName               = "MigrationAssessment.ovf"
	defaultIsoImageName          = "MigrationAssessment.iso"
	defaultRHCOSImage            = "rhcos-live.x86_64.iso"
)

type Proxy struct {
	HttpUrl       string
	HttpsUrl      string
	NoProxyDomain string
}

type ImageBuilder struct {
	SourceID                   string
	SshKey                     string
	Proxy                      Proxy
	CertificateChain           string
	PlannerServiceUI           string
	PlannerService             string
	MigrationPlannerAgentImage string
	InsecureRegistry           string
	Token                      string
	PersistentDiskDevice       string
	PersistentDiskImage        string
	IsoImageName               string
	OvfFile                    string
	OvfName                    string
	Template                   string
	RHCOSImage                 string
	imageType                  ImageType
}

func NewImageBuilder(sourceID uuid.UUID) *ImageBuilder {
	imageBuilder := &ImageBuilder{
		SourceID:                   sourceID.String(),
		PlannerService:             util.GetEnv("CONFIG_SERVER", defaultPlannerService),
		PlannerServiceUI:           util.GetEnv("CONFIG_SERVER_UI", defaultConfigServerUI),
		MigrationPlannerAgentImage: util.GetEnv("MIGRATION_PLANNER_AGENT_IMAGE", defaultAgentImage),
		PersistentDiskDevice:       util.GetEnv("PERSISTENT_DISK_DEVICE", defaultPersistenceDiskDevice),
		PersistentDiskImage:        defaultPersistentDiskImage,
		IsoImageName:               defaultIsoImageName,
		OvfFile:                    defaultOvfFile,
		OvfName:                    defaultOvfName,
		Template:                   defaultTemplate,
		RHCOSImage:                 defaultRHCOSImage,
		imageType:                  OVAImageType,
	}

	if insecureRegistry := os.Getenv("INSECURE_REGISTRY"); insecureRegistry != "" {
		imageBuilder.InsecureRegistry = insecureRegistry
	}

	return imageBuilder
}

func (b *ImageBuilder) Generate(ctx context.Context, w io.Writer) (int, error) {
	ignitionContent, err := b.generateIgnition()
	if err != nil {
		return -1, err
	}

	// Generate ISO data reader with ignition content
	reader, err := isoeditor.NewRHCOSStreamReader(b.RHCOSImage, &isoeditor.IgnitionContent{Config: []byte(ignitionContent)}, nil, nil)
	if err != nil {
		return -1, fmt.Errorf("error reading rhcos iso: %w", err)
	}

	size, err := b.computeSize(reader)
	if err != nil {
		return -1, err
	}

	// write only the iso in case of qemu
	if b.imageType == QemuImageType {
		if _, err := io.Copy(w, reader); err != nil {
			return 0, err
		}
		return size, nil
	}

	tw := tar.NewWriter(w)

	// OVF Must be first file in OVA, to support URL download
	if err := b.writeOvf(tw); err != nil {
		return -1, err
	}

	// Write ISO to TAR
	if err := b.writeIso(reader, tw); err != nil {
		return -1, err
	}

	if err := writePersistenceDisk(tw); err != nil {
		return -1, err
	}

	tw.Flush()

	return size, nil
}

func (b *ImageBuilder) Validate() error {
	ignitionContent, err := b.generateIgnition()
	if err != nil {
		return err
	}

	// Generate ISO data reader with ignition content
	if _, err = isoeditor.NewRHCOSStreamReader(b.RHCOSImage, &isoeditor.IgnitionContent{Config: []byte(ignitionContent)}, nil, nil); err != nil {
		return fmt.Errorf("error reading rhcos iso: %w", err)
	}

	return nil
}

func (b *ImageBuilder) generateIgnition() (string, error) {
	ignData := IgnitionData{
		SourceID:                   b.SourceID,
		SshKey:                     b.SshKey,
		PlannerServiceUI:           b.PlannerServiceUI,
		PlannerService:             b.PlannerService,
		MigrationPlannerAgentImage: b.MigrationPlannerAgentImage,
		InsecureRegistry:           b.InsecureRegistry,
		Token:                      b.Token,
		PersistentDiskDevice:       b.PersistentDiskDevice,
		HttpProxyUrl:               b.Proxy.HttpUrl,
		HttpsProxyUrl:              b.Proxy.HttpsUrl,
		NoProxyDomain:              b.Proxy.NoProxyDomain,
	}

	var buf bytes.Buffer
	t, err := template.New("ignition.template").ParseFiles(b.Template)
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

func (b *ImageBuilder) writeIso(reader overlay.OverlayReader, tw *tar.Writer) error {
	// Create a header for MigrationAssessment.iso
	length, err := reader.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	header := &tar.Header{
		Name:    b.IsoImageName,
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

func (b *ImageBuilder) writeOvf(tw *tar.Writer) error {
	ovfContent, err := os.ReadFile(b.OvfFile)
	if err != nil {
		return err
	}
	// Create a header for AgentVM.ovf
	header := &tar.Header{
		Name:    b.OvfName,
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

func (b *ImageBuilder) ovfSize() (int, error) {
	file, err := os.Open(b.OvfFile)
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

func (b *ImageBuilder) diskSize() (int, error) {
	file, err := os.Open(b.PersistentDiskImage)
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

func (b *ImageBuilder) computeSize(reader overlay.OverlayReader) (int, error) {
	length, err := reader.Seek(0, io.SeekEnd)
	if err != nil {
		return -1, err
	}

	// Reset the reader to start
	_, err = reader.Seek(0, io.SeekStart)
	if err != nil {
		return -1, err
	}

	isoSize := calculateTarSize(int(length))

	ovfSize, err := b.ovfSize()
	if err != nil {
		return -1, fmt.Errorf("failed to compute ovf size: %s", err)
	}

	persistentDiskSize, err := b.diskSize()
	if err != nil {
		return -1, fmt.Errorf("failed to comute persistent disk size: %s", err)
	}

	return isoSize + ovfSize + persistentDiskSize, nil
}

func (b *ImageBuilder) WithSshKey(sshKey string) *ImageBuilder {
	b.SshKey = sshKey
	return b
}

func (b *ImageBuilder) WithPlannerAgentImage(imageUrl string) *ImageBuilder {
	b.MigrationPlannerAgentImage = imageUrl
	return b
}

func (b *ImageBuilder) WithPlannerServiceUI(uiUrl string) *ImageBuilder {
	b.PlannerServiceUI = uiUrl
	return b
}

func (b *ImageBuilder) WithPlannerService(url string) *ImageBuilder {
	b.PlannerService = url
	return b
}

func (b *ImageBuilder) WithPersistenceDiskDevice(persistenceDevice string) *ImageBuilder {
	b.PersistentDiskDevice = persistenceDevice
	return b
}

func (b *ImageBuilder) WithAgentToken(token string) *ImageBuilder {
	b.Token = token
	return b
}

func (b *ImageBuilder) WithInsecureRegistry(insecureRegistry string) *ImageBuilder {
	b.InsecureRegistry = insecureRegistry
	return b
}

func (b *ImageBuilder) WithTemplate(templatePath string) *ImageBuilder {
	b.Template = templatePath
	return b
}

func (b *ImageBuilder) WithIsoImageName(name string) *ImageBuilder {
	b.IsoImageName = name
	return b
}

func (b *ImageBuilder) WithPersistentDiskImage(imagePath string) *ImageBuilder {
	b.PersistentDiskImage = imagePath
	return b
}

func (b *ImageBuilder) WithOvfFile(ovfFile string) *ImageBuilder {
	b.OvfFile = ovfFile
	return b
}

func (b *ImageBuilder) WithOvfName(ovfName string) *ImageBuilder {
	b.OvfName = ovfName
	return b
}

func (b *ImageBuilder) WithRHCOSImage(image string) *ImageBuilder {
	b.RHCOSImage = image
	return b
}

func (b *ImageBuilder) WithImageType(imageType ImageType) *ImageBuilder {
	b.imageType = imageType
	return b
}

func (b *ImageBuilder) WithProxy(proxy Proxy) *ImageBuilder {
	b.Proxy = proxy
	return b
}

func (b *ImageBuilder) WithCertificateChain(certs string) *ImageBuilder {
	b.CertificateChain = certs
	return b
}
