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
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/internal/util"
	"github.com/openshift/assisted-image-service/pkg/isoeditor"
	"github.com/openshift/assisted-image-service/pkg/overlay"
)

type Key int

// Key to store the ResponseWriter in the context of openapi
const ResponseWriterKey Key = 0

// Key to store the *http.Request in the context (needed for http.ServeContent)
const RequestKey Key = 1

type ImageType int

const (
	OVAImageType ImageType = iota
	QemuImageType
)

const (
	defaultPlannerService        = "http://127.0.0.1:7443"
	defaultPersistenceDiskDevice = "/dev/sda"
	defaultConfigServerUI        = "http://localhost:3000/migrate/wizard"
	defaultTemplate              = "data/ignition.template"
	defaultPersistentDiskImage   = "data/persistence-disk.vmdk"
	defaultOvfFile               = "data/MigrationAssessment.ovf"
	defaultOvfName               = "MigrationAssessment.ovf"
	defaultIsoImageName          = "MigrationAssessment.iso"
	defaultRHCOSImage            = "rhcos-live-iso.x86_64.iso"
)

// IgnitionData defines modifiable fields in ignition config
type IgnitionData struct {
	DebugMode            string
	SshKey               string
	PlannerServiceUI     string
	PlannerService       string
	InsecureRegistry     string
	Token                string
	PersistentDiskDevice string
	SourceID             string
	HttpProxyUrl         string
	HttpsProxyUrl        string
	NoProxyDomain        string
	RhcosPassword        string
	IpAddress            string
	SubnetMask           string
	DefaultGateway       string
	Dns                  string
}

type Proxy struct {
	HttpUrl       string
	HttpsUrl      string
	NoProxyDomain string
}

type VmNetwork struct {
	IpAddress      string
	SubnetMask     string
	DefaultGateway string
	Dns            string
}

type ImageBuilder struct {
	SourceID             string
	SshKey               string
	Proxy                Proxy
	CertificateChain     string
	DebugMode            string
	PlannerServiceUI     string
	PlannerService       string
	InsecureRegistry     string
	Token                string
	PersistentDiskDevice string
	PersistentDiskImage  string
	IsoImageName         string
	OvfFile              string
	OvfName              string
	Template             string
	RHCOSImage           string
	imageType            ImageType
	RhcosPassword        string
	VmNetwork            VmNetwork
}

func NewImageBuilder(sourceID uuid.UUID) *ImageBuilder {
	imageBuilder := &ImageBuilder{
		SourceID:             sourceID.String(),
		DebugMode:            util.GetEnv("DEBUG_MODE", ""),
		PlannerService:       util.GetEnv("CONFIG_SERVER", defaultPlannerService),
		PlannerServiceUI:     util.GetEnv("CONFIG_SERVER_UI", defaultConfigServerUI),
		PersistentDiskDevice: util.GetEnv("PERSISTENT_DISK_DEVICE", defaultPersistenceDiskDevice),
		PersistentDiskImage:  defaultPersistentDiskImage,
		IsoImageName:         defaultIsoImageName,
		OvfFile:              defaultOvfFile,
		OvfName:              defaultOvfName,
		Template:             defaultTemplate,
		RHCOSImage:           util.GetEnv("MIGRATION_PLANNER_ISO_PATH", defaultRHCOSImage),
		imageType:            OVAImageType,
	}

	if insecureRegistry := os.Getenv("INSECURE_REGISTRY"); insecureRegistry != "" {
		imageBuilder.InsecureRegistry = insecureRegistry
	}
	if rhcosPassword := os.Getenv("RHCOS_PASSWORD"); rhcosPassword != "" {
		imageBuilder.RhcosPassword = rhcosPassword
	}

	return imageBuilder
}

func (b *ImageBuilder) Size() (uint64, error) {
	ignitionContent, err := b.generateIgnition()
	if err != nil {
		return 0, err
	}

	// Generate ISO data reader with ignition content
	reader, err := isoeditor.NewRHCOSStreamReader(b.RHCOSImage, &isoeditor.IgnitionContent{Config: []byte(ignitionContent)}, nil, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to read rhcos iso: %w", err)
	}

	size, err := b.computeSize(reader)
	if err != nil {
		return 0, err
	}

	return size, nil
}

func (b *ImageBuilder) Generate(ctx context.Context, w io.Writer) error {
	ignitionContent, err := b.generateIgnition()
	if err != nil {
		return err
	}

	// Generate ISO data reader with ignition content
	reader, err := isoeditor.NewRHCOSStreamReader(b.RHCOSImage, &isoeditor.IgnitionContent{Config: []byte(ignitionContent)}, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to read rhcos iso: %w", err)
	}

	tw := tar.NewWriter(w)

	// OVF Must be first file in OVA, to support URL download
	if err := b.writeOvf(tw); err != nil {
		return err
	}

	// Write ISO to TAR
	if err := b.writeIso(reader, tw); err != nil {
		return err
	}

	if err := b.writePersistenceDisk(tw); err != nil {
		return err
	}

	_ = tw.Flush()

	return nil
}

// OpenSeekableReader returns an io.ReadSeeker over the OVA TAR content, along with
// the total size. This enables http.ServeContent to handle byte-range requests
// (required for Akamai LFO). The caller must call Close() on the returned reader.
// modTime is used for all TAR headers to ensure deterministic output across pods.
func (b *ImageBuilder) OpenSeekableReader(modTime time.Time) (*SeekableTarReader, int64, error) {
	ignitionContent, err := b.generateIgnition()
	if err != nil {
		return nil, 0, err
	}

	isoReader, err := isoeditor.NewRHCOSStreamReader(b.RHCOSImage, &isoeditor.IgnitionContent{Config: []byte(ignitionContent)}, nil, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read rhcos iso: %w", err)
	}

	// Ensure isoReader is closed on any error path below
	success := false
	defer func() {
		if !success {
			_ = isoReader.Close()
		}
	}()

	// Get ISO size via seeking
	isoSize, err := isoReader.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get iso size: %w", err)
	}
	if _, err := isoReader.Seek(0, io.SeekStart); err != nil {
		return nil, 0, fmt.Errorf("failed to reset iso reader: %w", err)
	}

	// Read OVF (small file, ~7 KB)
	ovfContent, err := os.ReadFile(b.OvfFile)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read ovf: %w", err)
	}

	// Read VMDK (small file, ~143 KB)
	diskContent, err := os.ReadFile(b.PersistentDiskImage)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read persistence disk: %w", err)
	}

	entries := []TarEntry{
		{
			Name:    b.OvfName,
			Size:    int64(len(ovfContent)),
			Mode:    0600,
			ModTime: modTime,
			Reader:  bytes.NewReader(ovfContent),
		},
		{
			Name:    b.IsoImageName,
			Size:    isoSize,
			Mode:    0600,
			ModTime: modTime,
			Reader:  isoReader,
		},
		{
			Name:    "persistence-disk.vmdk",
			Size:    int64(len(diskContent)),
			Mode:    0600,
			ModTime: modTime,
			Reader:  bytes.NewReader(diskContent),
		},
	}

	reader, total, err := NewSeekableTarReader(entries, isoReader)
	if err != nil {
		return nil, 0, err
	}
	success = true
	return reader, total, nil
}

func (b *ImageBuilder) Validate() error {
	ignitionContent, err := b.generateIgnition()
	if err != nil {
		return err
	}

	// Generate ISO data reader with ignition content
	if _, err = isoeditor.NewRHCOSStreamReader(b.RHCOSImage, &isoeditor.IgnitionContent{Config: []byte(ignitionContent)}, nil, nil); err != nil {
		return fmt.Errorf("failed to read rhcos iso: %w", err)
	}

	return nil
}

func (b *ImageBuilder) generateIgnition() (string, error) {
	ignData := IgnitionData{
		DebugMode:            b.DebugMode,
		SourceID:             b.SourceID,
		SshKey:               b.SshKey,
		PlannerServiceUI:     b.PlannerServiceUI,
		PlannerService:       b.PlannerService,
		InsecureRegistry:     b.InsecureRegistry,
		Token:                b.Token,
		PersistentDiskDevice: b.PersistentDiskDevice,
		HttpProxyUrl:         b.Proxy.HttpUrl,
		HttpsProxyUrl:        b.Proxy.HttpsUrl,
		NoProxyDomain:        b.Proxy.NoProxyDomain,
		RhcosPassword:        b.RhcosPassword,
		IpAddress:            b.VmNetwork.IpAddress,
		SubnetMask:           b.VmNetwork.SubnetMask,
		DefaultGateway:       b.VmNetwork.DefaultGateway,
		Dns:                  b.VmNetwork.Dns,
	}

	var buf bytes.Buffer
	t, err := template.New("ignition.template").ParseFiles(b.Template)
	if err != nil {
		return "", fmt.Errorf("failed to read the ignition template: %w", err)
	}
	if err := t.Execute(&buf, ignData); err != nil {
		return "", fmt.Errorf("failed to parse the ignition template: %w", err)
	}

	dataOut, _, err := config.TranslateBytes(buf.Bytes(), common.TranslateBytesOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to translate config: %w", err)
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

func (b *ImageBuilder) ovfSize() (uint64, error) {
	file, err := os.Open(b.OvfFile)
	if err != nil {
		return 0, err
	}
	defer func() { _ = file.Close() }()
	fileInfo, err := file.Stat()
	if err != nil {
		return 0, err
	}

	return b.calculateTarSize(uint64(fileInfo.Size())), nil
}

func (b *ImageBuilder) diskSize() (uint64, error) {
	file, err := os.Open(b.PersistentDiskImage)
	if err != nil {
		return 0, err
	}
	defer func() { _ = file.Close() }()
	fileInfo, err := file.Stat()
	if err != nil {
		return 0, err
	}

	return b.calculateTarSize(uint64(fileInfo.Size())), nil
}

func (b *ImageBuilder) computeSize(reader overlay.OverlayReader) (uint64, error) {
	length, err := reader.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, err
	}

	// Reset the reader to start
	_, err = reader.Seek(0, io.SeekStart)
	if err != nil {
		return 0, err
	}

	isoSize := b.calculateTarSize(uint64(length))

	ovfSize, err := b.ovfSize()
	if err != nil {
		return 0, fmt.Errorf("failed to compute ovf size: %s", err)
	}

	persistentDiskSize, err := b.diskSize()
	if err != nil {
		return 0, fmt.Errorf("failed to comute persistent disk size: %s", err)
	}

	// 1024 bytes for the two 512-byte zero end-of-archive blocks
	const endOfArchiveSize = 1024
	return isoSize + ovfSize + persistentDiskSize + endOfArchiveSize, nil
}

func (b *ImageBuilder) WithImageInfra(imageInfra model.ImageInfra) *ImageBuilder {
	if imageInfra.SshPublicKey != "" {
		b.WithSshKey(imageInfra.SshPublicKey)
	}

	if imageInfra.HttpProxyUrl != "" || imageInfra.HttpsProxyUrl != "" || imageInfra.NoProxyDomains != "" {
		b.WithProxy(
			Proxy{
				HttpUrl:       imageInfra.HttpProxyUrl,
				HttpsUrl:      imageInfra.HttpsProxyUrl,
				NoProxyDomain: imageInfra.NoProxyDomains,
			},
		)
	}

	if imageInfra.CertificateChain != "" {
		b.WithCertificateChain(imageInfra.CertificateChain)
	}

	if imageInfra.IpAddress != "" {
		b.WithVmNetwork(VmNetwork{
			IpAddress:      imageInfra.IpAddress,
			SubnetMask:     imageInfra.SubnetMask,
			DefaultGateway: imageInfra.DefaultGateway,
			Dns:            imageInfra.Dns,
		})
	}

	return b
}

func (b *ImageBuilder) WithSshKey(sshKey string) *ImageBuilder {
	b.SshKey = sshKey
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

func (b *ImageBuilder) WithVmNetwork(network VmNetwork) *ImageBuilder {
	b.VmNetwork = network
	return b
}

func (b *ImageBuilder) calculateTarSize(contentSize uint64) uint64 {
	const blockSize uint64 = 512

	// Size of the tar header block
	size := blockSize

	// Size of the file content, rounded up to nearest 512 bytes
	size += ((contentSize + blockSize - 1) / blockSize) * blockSize

	return size
}

func (b *ImageBuilder) writePersistenceDisk(tw *tar.Writer) error {
	diskContent, err := os.ReadFile("data/persistence-disk.vmdk")
	if err != nil {
		return err
	}

	header := &tar.Header{
		Name:    "persistence-disk.vmdk",
		Size:    int64(len(diskContent)),
		Mode:    0600,
		ModTime: time.Now(),
	}

	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	if _, err := tw.Write([]byte(diskContent)); err != nil {
		return err
	}

	return nil
}
