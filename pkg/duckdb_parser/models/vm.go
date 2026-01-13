package models

// VM represents a virtual machine from VMware inventory.
type VM struct {
	ID                       string   `db:"VM ID"`                                  // vinfo
	Name                     string   `db:"VM"`                                     // vinfo
	Folder                   string   `db:"Folder ID"`                              // vinfo
	Host                     string   `db:"Host"`                                   // vinfo
	UUID                     string   `db:"SMBIOS UUID"`                            // vinfo
	Firmware                 string   `db:"Firmware"`                               // vinfo
	PowerState               string   `db:"Powerstate"`                             // vinfo
	ConnectionState          string   `db:"Connection state"`                       // vinfo
	CpuHotAddEnabled         bool     `db:"Hot Add"`                                // vcpu
	CpuHotRemoveEnabled      bool     `db:"Hot Remove"`                             // vcpu
	MemoryHotAddEnabled      bool     `db:"Hot Add"`                                // vmemory
	FaultToleranceEnabled    bool     `db:"FT State"`                               // vinfo
	CpuCount                 int32    `db:"CPUs"`                                   // vinfo
	CpuSockets               int32    `db:"Sockets"`                                // vcpu
	CoresPerSocket           int32    `db:"Cores p/s"`                              // vcpu
	MemoryMB                 int32    `db:"Memory"`                                 // vinfo
	GuestName                string   `db:"OS according to the configuration file"` // vinfo
	GuestNameFromVmwareTools string   `db:"OS according to the VMware Tools"`       // vinfo
	HostName                 string   `db:"DNS Name"`                               // vinfo
	BalloonedMemory          int32    `db:"Ballooned"`                              // vmemory
	IpAddress                string   `db:"Primary IP Address"`                     // vinfo
	StorageUsed              int32    `db:"In Use MiB"`                             // vinfo
	IsTemplate               bool     `db:"Template"`                               // vinfo
	ChangeTrackingEnabled    bool     `db:"CBT"`                                    // vinfo
	NICs                     NICs     //                                               vnetwork
	Disks                    Disks    //                                               vdisk
	Networks                 Networks `db:"network_object_id"`       // vinfo (Network #1, #2, etc.)
	DiskEnableUuid           bool     `db:"EnableUUID"`              // vinfo
	Datacenter               string   `db:"Datacenter"`              // vinfo
	Cluster                  string   `db:"Cluster"`                 // vinfo
	HWVersion                string   `db:"HW version"`              // vinfo
	TotalDiskCapacityMiB     int32    `db:"Total disk capacity MiB"` // vinfo
	ProvisionedMiB           int32    `db:"Provisioned MiB"`         // vinfo
	ResourcePool             string   `db:"Resource pool"`           // vinfo
	Concerns                 Concerns // concerns table (via LEFT JOIN)
}

// Disk represents a virtual disk from the vdisk table.
type Disk struct {
	Key           string `db:"Disk Key"` // vdisk
	UnitNumber    string `db:"Unit #"`   // vdisk
	ControllerKey int32  //                      derived
	File          string `db:"Path"`         // vdisk
	Capacity      int64  `db:"Capacity MiB"` // vdisk
	Shared        bool   `db:"Sharing mode"` // vdisk
	RDM           bool   `db:"Raw"`          // vdisk
	Bus           string `db:"Shared Bus"`   // vdisk
	Mode          string `db:"Disk Mode"`    // vdisk
	Serial        string `db:"Disk UUID"`    // vdisk
	Thin          string `db:"Thin"`         // vdisk
	Controller    string `db:"Controller"`   // vdisk
	Label         string `db:"Label"`        // vdisk
	SCSIUnit      string `db:"SCSI Unit #"`  // vdisk
}

// NIC represents a network interface from the vnetwork table.
type NIC struct {
	Network         string `db:"Network"`          // vnetwork
	MAC             string `db:"Mac Address"`      // vnetwork
	Label           string `db:"NIC label"`        // vnetwork
	Adapter         string `db:"Adapter"`          // vnetwork
	Switch          string `db:"Switch"`           // vnetwork
	Connected       bool   `db:"Connected"`        // vnetwork
	StartsConnected bool   `db:"Starts Connected"` // vnetwork
	Type            string `db:"Type"`             // vnetwork
	IPv4Address     string `db:"IPv4 Address"`     // vnetwork
	IPv6Address     string `db:"IPv6 Address"`     // vnetwork
}
