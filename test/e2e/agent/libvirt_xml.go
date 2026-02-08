package agent

import "encoding/xml"

// ---------- Root ----------

type Domain struct {
	XMLName xml.Name `xml:"domain"`
	Type    string   `xml:"type,attr"`

	Name     string   `xml:"name"`
	Memory   Memory   `xml:"memory"`
	VCPU     VCPU     `xml:"vcpu"`
	Metadata Metadata `xml:"metadata"`
	OS       OS       `xml:"os"`
	CPU      CPU      `xml:"cpu"`
	Features Features `xml:"features"`
	Devices  Devices  `xml:"devices"`
}

// ---------- Basic fields ----------

type Memory struct {
	Unit  string `xml:"unit,attr"`
	Value int    `xml:",chardata"`
}

type VCPU struct {
	Placement string `xml:"placement,attr"`
	Value     int    `xml:",chardata"`
}

// ---------- Metadata / Namespace ----------

type Metadata struct {
	LibOSInfo LibOSInfo `xml:"libosinfo:libosinfo"`
}

type LibOSInfo struct {
	XMLName xml.Name `xml:"libosinfo:libosinfo"`
	Xmlns   string   `xml:"xmlns:libosinfo,attr"`
	OS      LibOS    `xml:"libosinfo:os"`
}

type LibOS struct {
	ID string `xml:"id,attr"`
}

// ---------- OS ----------

type OS struct {
	Type Type `xml:"type"`
	Boot Boot `xml:"boot"`
}

type Type struct {
	Arch    string `xml:"arch,attr"`
	Machine string `xml:"machine,attr"`
	Value   string `xml:",chardata"`
}

type Boot struct {
	Dev string `xml:"dev,attr"`
}

// ---------- CPU / Features ----------

type CPU struct {
	Mode       string `xml:"mode,attr"`
	Check      string `xml:"check,attr"`
	Migratable string `xml:"migratable,attr"`
}

type Features struct {
	ACPI struct{} `xml:"acpi"`
	APIC struct{} `xml:"apic"`
}

// ---------- Devices ----------

type Devices struct {
	Emulator  string    `xml:"emulator"`
	Disks     []Disk    `xml:"disk"`
	Interface Interface `xml:"interface"`
	Graphics  Graphics  `xml:"graphics"`
	Console   Console   `xml:"console"`
}

type Disk struct {
	Type     string    `xml:"type,attr"`
	Device   string    `xml:"device,attr"`
	Driver   Driver    `xml:"driver"`
	Source   Source    `xml:"source"`
	Target   Target    `xml:"target"`
	Readonly *struct{} `xml:"readonly,omitempty"`
}

type Driver struct {
	Name string `xml:"name,attr"`
	Type string `xml:"type,attr"`
}

type Source struct {
	File string `xml:"file,attr"`
}

type Target struct {
	Dev string `xml:"dev,attr"`
	Bus string `xml:"bus,attr"`
}

type Interface struct {
	Type   string    `xml:"type,attr"`
	Source NetSource `xml:"source"`
	Model  Model     `xml:"model"`
}

type NetSource struct {
	Network string `xml:"network,attr"`
}

type Model struct {
	Type string `xml:"type,attr"`
}

type Graphics struct {
	Type string `xml:"type,attr"`
	Port string `xml:"port,attr"`
}

type Console struct {
	Type string `xml:"type,attr"`
}

// ---------- Generator function ----------

func GenerateDomainXML(vmName, isoPath, diskPath string) ([]byte, error) {
	domain := Domain{
		Type: "kvm",
		Name: vmName,
		Memory: Memory{
			Unit:  "MiB",
			Value: 4096,
		},
		VCPU: VCPU{
			Placement: "static",
			Value:     2,
		},
		Metadata: Metadata{
			LibOSInfo: LibOSInfo{
				Xmlns: "http://libosinfo.org/xmlns/libvirt/domain/1.0",
				OS: LibOS{
					ID: "http://fedoraproject.org/coreos/stable",
				},
			},
		},
		OS: OS{
			Type: Type{
				Arch:    "x86_64",
				Machine: "pc-q35-6.2",
				Value:   "hvm",
			},
			Boot: Boot{
				Dev: "cdrom",
			},
		},
		CPU: CPU{
			Mode:       "host-passthrough",
			Check:      "none",
			Migratable: "on",
		},
		Features: Features{},
		Devices: Devices{
			Emulator: "/usr/bin/qemu-system-x86_64",
			Disks: []Disk{
				{
					Type:   "file",
					Device: "disk",
					Driver: Driver{Name: "qemu", Type: "qcow2"},
					Source: Source{File: diskPath},
					Target: Target{Dev: "vda", Bus: "virtio"},
				},
				{
					Type:     "file",
					Device:   "cdrom",
					Driver:   Driver{Name: "qemu", Type: "raw"},
					Source:   Source{File: isoPath},
					Target:   Target{Dev: "sda", Bus: "sata"},
					Readonly: &struct{}{},
				},
			},
			Interface: Interface{
				Type:   "network",
				Source: NetSource{Network: "default"},
				Model:  Model{Type: "virtio"},
			},
			Graphics: Graphics{
				Type: "vnc",
				Port: "-1",
			},
			Console: Console{
				Type: "pty",
			},
		},
	}

	return xml.MarshalIndent(domain, "", "  ")
}
