package cli

import (
	"context"
	"fmt"
	"html/template"
	"os"
	"path"
	"strings"

	"github.com/google/uuid"
	libvirt "github.com/libvirt/libvirt-go"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const libvirtDomainDefinitionTemplate = `
<domain type='kvm'>
  <name>{{ .Name }}</name>
  <memory unit='MiB'>4096</memory>
  <vcpu placement='static'>2</vcpu>
  <metadata>
     <libosinfo:libosinfo xmlns:libosinfo="http://libosinfo.org/xmlns/libvirt/domain/1.0">
       <libosinfo:os id="http://fedoraproject.org/coreos/stable"/>
     </libosinfo:libosinfo>
  </metadata>
  <os>
    <type arch='x86_64' machine='pc-q35-6.2'>hvm</type>
    <boot dev='cdrom'/>
  </os>
  <cpu mode='host-passthrough' check='none' migratable='on'/>
  <features>
    <acpi/>
    <apic/>
  </features>
  <devices>
    <emulator>/usr/bin/qemu-system-x86_64</emulator>
	<disk type='volume' device='disk'>
        <driver name='qemu' type='qcow2'/>
    	<source pool='{{ .StoragePool }}' volume='{{ .Volume }}'/>
        <target dev='vda' bus='virtio'/>
	</disk>
    <disk type='file' device='cdrom'>
      <driver name='qemu' type='raw'/>
      <source file='{{ .ImagePath }}'/>
      <target dev='sda' bus='sata'/>
      <readonly/>
    </disk>
    <interface type='network'>
      <source network='{{ .Network }}'/>
      <model type='virtio'/>
    </interface>
    <graphics type='vnc' port='-1'/>
    <console type='pty'/>
  </devices>
</domain>
`

const persistenceVolDefinitionTemplate = `
<volume>
  <name>{{ .Name }}</name>
  <allocation>0</allocation>
  <capacity unit="Gb">1</capacity>
  <target>
  	<format type="qcow2" />
  </target>
</volume>
`

type domainParameters struct {
	PersistenceFilePath string
	ImagePath           string
	Network             string
	Name                string
	StoragePool         string
	Volume              string
}

type persistenceVolumeParameters struct {
	Name string
}

type DeployOptions struct {
	GlobalOptions

	ImageFile   string
	Name        string
	NetworkName string
	QemuUrl     string
	StoragePool string
}

func DefaultDeployOptions() *DeployOptions {
	return &DeployOptions{
		GlobalOptions: DefaultGlobalOptions(),
	}
}

func NewCmdDeploy() *cobra.Command {
	o := DefaultDeployOptions()
	cmd := &cobra.Command{
		Use:     "deploy SOURCE_ID [FLAGS]",
		Short:   "Deploy an agent",
		Example: "deploy <source_id> -s ~/.ssh/some_key.pub --name agent_vm --network bridge",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := o.Complete(cmd, args); err != nil {
				return err
			}
			if err := o.Validate(args); err != nil {
				return err
			}
			return o.Run(cmd.Context(), args)
		},
		SilenceUsage: true,
	}
	o.Bind(cmd.Flags())
	return cmd
}

func (o *DeployOptions) Bind(fs *pflag.FlagSet) {
	o.GlobalOptions.Bind(fs)

	fs.StringVarP(&o.ImageFile, "image-file", "", o.ImageFile, "Path the iso image. If not set the image will be generated with default values.")
	fs.StringVarP(&o.Name, "name", "", o.Name, "Name of the vm")
	fs.StringVarP(&o.NetworkName, "network", "", "default", "Name of the network")
	fs.StringVarP(&o.QemuUrl, "qemu-url", "", "qemu:///session", "Url of qemu")
	fs.StringVarP(&o.StoragePool, "storage-pool", "", "default", "Name of the storage pool")
}

func (o *DeployOptions) Validate(args []string) error {
	if _, err := uuid.Parse(args[0]); err != nil {
		return fmt.Errorf("invalid source id: %s", err)
	}

	if o.Name == "" {
		// generate a vm like agent-123456
		o.Name = fmt.Sprintf("agent-%s", uuid.NewString()[:6])
	}

	return nil
}

func (o *DeployOptions) Run(ctx context.Context, args []string) error {
	if o.ImageFile == "" {
		tmpFolder, err := os.MkdirTemp("", o.Name)
		if err != nil {
			return fmt.Errorf("failed to create temporary folder for image iso: %v", err)
		}
		o.ImageFile = path.Join(tmpFolder, "image.iso")

		generateO := DefaultGenerateOptions()
		generateO.ImageType = "iso"
		generateO.OutputImageFilePath = o.ImageFile

		if err := generateO.Validate(args); err != nil {
			return err
		}

		if err := generateO.Run(ctx, args); err != nil {
			return err
		}
	}

	conn, err := libvirt.NewConnect(o.QemuUrl)
	if err != nil {
		return fmt.Errorf("failed to connect to hypervisor: %s", err)
	}
	defer func() {
		conn.Close()
	}()

	// try to find the storage pool
	storagePool, err := conn.LookupStoragePoolByName(o.StoragePool)
	if err != nil {
		return fmt.Errorf("failed to find storage pool %s: %s", o.StoragePool, err)
	}

	volumeDef, err := generateTemplate(persistenceVolDefinitionTemplate, persistenceVolumeParameters{
		Name: fmt.Sprintf("persistent-vol-%s", o.Name),
	})
	if err != nil {
		return fmt.Errorf("failed to generate volume domain definition: %s", err)
	}

	// generate persistence volume
	volume, err := storagePool.StorageVolCreateXML(volumeDef, libvirt.STORAGE_VOL_CREATE_PREALLOC_METADATA)
	if err != nil {
		return fmt.Errorf("failed to create persistence volume: %s", err)
	}

	volumeName, err := volume.GetName()
	if err != nil {
		return fmt.Errorf("failed to get volume name: %s", err)
	}

	// create domain defintion
	domDefinition, err := generateTemplate(libvirtDomainDefinitionTemplate, domainParameters{
		ImagePath:   o.ImageFile,
		Network:     o.NetworkName,
		Name:        o.Name,
		Volume:      volumeName,
		StoragePool: o.StoragePool,
	})
	if err != nil {
		return fmt.Errorf("failed to generate libvirt domain definition: %s", err)
	}

	domain, err := conn.DomainDefineXML(domDefinition)
	if err != nil {
		return fmt.Errorf("failed to define domain: %v", err)
	}
	defer func() {
		_ = domain.Free()
	}()

	// Start the domain
	if err := domain.Create(); err != nil {
		return fmt.Errorf("failed to create domain: %v", err)
	}

	fmt.Printf("agent started: %s", o.Name)

	return nil
}

func generateTemplate(defTemplate string, data any) (string, error) {
	// create domain defintion
	tmpl, err := template.New("template").Parse(defTemplate)
	if err != nil {
		return "", err
	}

	var defBuilder strings.Builder
	if err := tmpl.Execute(&defBuilder, data); err != nil {
		return "", err
	}

	return defBuilder.String(), nil
}
