package models

// Datastore represents a VMware datastore.
type Datastore struct {
	Cluster                 string  `db:"cluster" json:"-"`
	DiskId                  string  `db:"diskId" json:"diskId"`
	FreeCapacityGB          float64 `db:"freeCapacityGB" json:"freeCapacityGB"`
	HardwareAcceleratedMove bool    `db:"hardwareAcceleratedMove" json:"hardwareAcceleratedMove"`
	HostId                  string  `db:"hostId" json:"hostId"`
	Model                   string  `db:"model" json:"model"`
	ProtocolType            string  `db:"protocolType" json:"protocolType"`
	TotalCapacityGB         float64 `db:"totalCapacityGB" json:"totalCapacityGB"`
	Type                    string  `db:"type" json:"type"`
	Vendor                  string  `db:"vendor" json:"vendor"`
}

// Os represents an operating system summary.
type Os struct {
	Name      string `db:"name" json:"name"`
	Count     int    `db:"count" json:"count"`
	Supported bool   `db:"supported" json:"supported"`
}

// Host represents a VMware ESXi host.
type Host struct {
	Cluster    string `db:"cluster" json:"-"`
	CpuCores   int    `db:"cpuCores" json:"cpuCores"`
	CpuSockets int    `db:"cpuSockets" json:"cpuSockets"`
	Id         string `db:"id" json:"id"`
	MemoryMB   int    `db:"memoryMB" json:"memoryMB"`
	Model      string `db:"model" json:"model"`
	Vendor     string `db:"vendor" json:"vendor"`
}

// Network represents a VMware network.
type Network struct {
	Cluster  string `db:"cluster" json:"-"`
	Dvswitch string `db:"dvswitch" json:"dvswitch"`
	Name     string `db:"name" json:"name"`
	Type     string `db:"type" json:"type"`
	VlanId   string `db:"vlanId" json:"vlanId"`
	VmsCount int    `db:"vmsCount" json:"vmsCount"`
}

// Cluster represents a VMware cluster with its resources.
type Cluster struct {
	Name       string      `json:"name"`
	Datastores []Datastore `json:"datastores"`
	Hosts      []Host      `json:"hosts"`
	Networks   []Network   `json:"networks"`
}
