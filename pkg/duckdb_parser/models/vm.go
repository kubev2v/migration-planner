package models

// Ref represents an object reference with ID.
type Ref struct {
	ID string `json:"id"`
}

// Metadata maps metadata keys to their values for one VM.
type Metadata map[string][]string

// VM represents a virtual machine from VMware inventory.
type VM struct {
	ID                       string    `json:"id" db:"VM ID"`
	Name                     string    `json:"name" db:"VM"`
	Folder                   string    `json:"folder" db:"Folder ID"`
	Host                     string    `json:"host" db:"Host"`
	UUID                     string    `json:"uuid" db:"SMBIOS UUID"`
	Firmware                 string    `json:"firmware" db:"Firmware"`
	PowerState               string    `json:"powerState" db:"Powerstate"`
	ConnectionState          string    `json:"connectionState" db:"Connection state"`
	CpuHotAddEnabled         bool      `json:"cpuHotAddEnabled" db:"Hot Add"`       // vcpu
	CpuHotRemoveEnabled      bool      `json:"cpuHotRemoveEnabled" db:"Hot Remove"` // vcpu
	MemoryHotAddEnabled      bool      `json:"memoryHotAddEnabled" db:"Hot Add"`    // vmemory
	FaultToleranceEnabled    bool      `json:"faultToleranceEnabled" db:"FT State"` // vinfo
	CpuCount                 int32     `json:"cpuCount" db:"CPUs"`                  // vinfo
	CpuSockets               int32     `json:"cpuSockets" db:"Sockets"`             // vcpu
	CoresPerSocket           int32     `json:"coresPerSocket" db:"Cores p/s"`       // vcpu
	MemoryMB                 int32     `json:"memoryMB" db:"Memory"`                // vinfo
	GuestName                string    `json:"guestName" db:"OS according to the configuration file"`
	GuestNameFromVmwareTools string    `json:"guestNameFromVmwareTools" db:"OS according to the VMware Tools"`
	HostName                 string    `json:"hostName" db:"DNS Name"`
	BalloonedMemory          int32     `json:"balloonedMemory" db:"Ballooned"` // vmemory
	IpAddress                string    `json:"ipAddress" db:"Primary IP Address"`
	StorageUsed              int64     `json:"storageUsed" db:"In Use MiB"` // SQL returns bytes
	IsTemplate               bool      `json:"isTemplate" db:"Template"`
	ChangeTrackingEnabled    bool      `json:"changeTrackingEnabled" db:"CBT"`
	NICs                     NICs      `json:"nics"`
	Disks                    Disks     `json:"disks"`
	Networks                 Networks  `json:"networks" db:"network_object_id"`
	DiskEnableUuid           bool      `json:"diskEnableUuid" db:"EnableUUID"`
	Datacenter               string    `json:"datacenter" db:"Datacenter"`
	Cluster                  string    `json:"cluster" db:"Cluster"`
	HWVersion                string    `json:"hwVersion" db:"HW version"`
	TotalDiskCapacityMiB     int32     `json:"totalDiskCapacityMiB" db:"Total disk capacity MiB"`
	ProvisionedMiB           int32     `json:"provisionedMiB" db:"Provisioned MiB"`
	ResourcePool             string    `json:"resourcePool" db:"Resource pool"`
	OsDiskComplexity         int32     `json:"osDiskComplexity" db:"OsDiskComplexity"`
	Concerns                 Concerns  `json:"concerns"`
	NumaNodeAffinity         []string  `json:"numaNodeAffinity"` // Always empty for RVTools, included for OPA compatibility
	MigrationExcluded        bool      `json:"migrationExcluded" db:"migration_excluded"`
	Labels                   Labels    `json:"labels" db:"labels"`
	GuestApps                GuestApps `json:"guestApps" db:"guest_apps"`
	Metadata                 Metadata  `json:"metadata" db:"metadata"`
}

// GuestApp represents a guest application installed on a VM.
type GuestApp struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// EffectiveGuestName returns the best available guest OS name.
// Prefers VMware Tools detection over configuration file.
func (vm VM) EffectiveGuestName() string {
	if vm.GuestNameFromVmwareTools != "" {
		return vm.GuestNameFromVmwareTools
	}
	return vm.GuestName
}

// RDMBacking identifies the LUN directly reported by an RDM disk backing.
type RDMBacking struct {
	LunUUID string `json:"lunUuid,omitempty"`
}

// Disk represents a virtual disk from the vdisk table.
type Disk struct {
	Key                             string      `json:"key" db:"Disk Key"`
	UnitNumber                      int32       `json:"unitNumber" db:"Unit #"` // Changed to int32 for OPA
	ControllerKey                   int32       `json:"controllerKey"`          // Populated by scanner using ControllerTracker
	File                            string      `json:"file" db:"Path"`
	Capacity                        int64       `json:"capacity" db:"Capacity MiB"` // SQL returns bytes
	Shared                          bool        `json:"shared" db:"Sharing mode"`
	RDM                             bool        `json:"rdm" db:"Raw"`
	RDMBacking                      *RDMBacking `json:"rdmBacking,omitempty"`
	Bus                             string      `json:"bus" db:"Shared Bus"` // Populated by scanner using ControllerTracker
	Mode                            string      `json:"mode,omitempty" db:"Disk Mode"`
	Serial                          string      `json:"serial" db:"Disk UUID"`
	Thin                            string      `json:"thin" db:"Thin"`
	Controller                      string      `json:"controller" db:"Controller"`
	Label                           string      `json:"label" db:"Label"`
	SCSIUnit                        string      `json:"scsiUnit" db:"SCSI Unit #"`
	Datastore                       Ref         `json:"datastore"` // Changed from string to Ref
	DatastoreName                   string      `json:"datastoreName,omitempty"`
	DatastoreType                   string      `json:"datastoreType,omitempty"`
	DatastoreFilesystemType         string      `json:"datastoreFilesystemType,omitempty"`
	DatastoreFilesystemMajorVersion int32       `json:"datastoreFilesystemMajorVersion,omitempty"`
	DatastoreStorageProtocol        string      `json:"datastoreStorageProtocol,omitempty"`
	DatastoreBackingDeviceIDs       []string    `json:"datastoreBackingDeviceIds,omitempty"`
	DatastoreVendor                 string      `json:"datastoreVendor,omitempty"`
	DatastoreModel                  string      `json:"datastoreModel,omitempty"`
	ChangeTrackingEnabled           bool        `json:"changeTrackingEnabled"` // Inherited from VM for OPA per-disk CBT check
}

// NIC represents a network interface from the vnetwork table.
type NIC struct {
	Network         Ref    `json:"network"` // Changed from string to Ref
	MAC             string `json:"mac" db:"Mac Address"`
	Label           string `json:"label" db:"NIC label"`
	Adapter         string `json:"adapter" db:"Adapter"`
	Switch          string `json:"switch" db:"Switch"`
	Connected       bool   `json:"connected" db:"Connected"`
	StartsConnected bool   `json:"startsConnected" db:"Starts Connected"`
	Type            string `json:"type" db:"Type"`
	IPv4Address     string `json:"ipv4Address" db:"IPv4 Address"`
	IPv6Address     string `json:"ipv6Address" db:"IPv6 Address"`
}
