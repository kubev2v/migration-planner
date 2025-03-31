package store

import (
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
)

func GenerateDefaultInventory() api.Inventory {
	n := 107
	dvSwitch := "management"
	dvSwitchVm := "vm"
	vlanid100 := "100"
	vlanid200 := "200"
	vlanTrunk := "0-4094"
	return api.Inventory{
		Infra: api.Infra{
			Datastores: []api.Datastore{
				{FreeCapacityGB: 615, TotalCapacityGB: 766, Type: "VMFS", Vendor: "NETAPP", DiskId: "naa.600a098038314648593f517773636465"},
				{FreeCapacityGB: 650, TotalCapacityGB: 766, Type: "VMFS", Vendor: "3PARdata", DiskId: "naa.600a098038314648593f517773636465"},
				{FreeCapacityGB: 167, TotalCapacityGB: 221, Type: "VMFS", Vendor: "3PARdata", DiskId: "naa.600a098038314648593f517773636465"},
				{FreeCapacityGB: 424, TotalCapacityGB: 766, Type: "VMFS", Vendor: "NETAPP", DiskId: "naa.600a098038314648593f517773636465"},
				{FreeCapacityGB: 1369, TotalCapacityGB: 3321, Type: "VMFS", Vendor: "ATA", DiskId: "N/A"},
				{FreeCapacityGB: 1252, TotalCapacityGB: 3071, Type: "VMFS", Vendor: "ATA", DiskId: "N/A"},
				{FreeCapacityGB: 415, TotalCapacityGB: 766, Type: "VMFS", Vendor: "ATA", DiskId: "N/A"},
				{FreeCapacityGB: 585, TotalCapacityGB: 766, Type: "VMFS", Vendor: "ATA", DiskId: "N/A"},
				{FreeCapacityGB: 170, TotalCapacityGB: 196, Type: "NFS", Vendor: "N/A", DiskId: "N/A"},
				{FreeCapacityGB: 606, TotalCapacityGB: 766, Type: "VMFS", Vendor: "ATA", DiskId: "N/A"},
				{FreeCapacityGB: 740, TotalCapacityGB: 766, Type: "VMFS", Vendor: "ATA", DiskId: "N/A"},
			},
			HostPowerStates: map[string]int{
				"Green": 8,
			},
			HostsPerCluster: []int{1, 7, 0},
			Networks: []struct {
				Dvswitch *string               `json:"dvswitch,omitempty"`
				Name     string                `json:"name"`
				Type     api.InfraNetworksType `json:"type"`
				VlanId   *string               `json:"vlanId,omitempty"`
			}{
				{Name: dvSwitch, Type: "dvswitch"},
				{Dvswitch: &dvSwitch, Name: "mgmt", Type: "distributed", VlanId: &vlanid100},
				{Name: dvSwitchVm, Type: "dvswitch"},
				{Dvswitch: &dvSwitchVm, Name: "storage", Type: "distributed", VlanId: &vlanid200},
				{Dvswitch: &dvSwitch, Name: "vMotion", Type: "distributed", VlanId: &vlanid100},
				{Dvswitch: &dvSwitch, Name: "trunk", Type: "distributed", VlanId: &vlanTrunk},
			},
			TotalClusters: 3,
			TotalHosts:    8,
		},
		Vcenter: api.VCenter{
			Id: "00000000-0000-0000-0000-000000000000",
		},
		Vms: api.VMs{
			CpuCores: api.VMResourceBreakdown{
				Histogram: struct {
					Data     []int `json:"data"`
					MinValue int   `json:"minValue"`
					Step     int   `json:"step"`
				}{
					Data:     []int{45, 0, 39, 2, 13, 0, 0, 0, 0, 8},
					MinValue: 1,
					Step:     2,
				},
				Total:                          472,
				TotalForMigratableWithWarnings: 472,
			},
			DiskCount: api.VMResourceBreakdown{
				Histogram: struct {
					Data     []int `json:"data"`
					MinValue int   `json:"minValue"`
					Step     int   `json:"step"`
				}{
					Data:     []int{10, 91, 1, 2, 0, 2, 0, 0, 0, 1},
					MinValue: 0,
					Step:     1,
				},
				Total:                          115,
				TotalForMigratableWithWarnings: 115,
			},
			NotMigratableReasons: []struct {
				Assessment string `json:"assessment"`
				Count      int    `json:"count"`
				Label      string `json:"label"`
			}{},
			DiskGB: api.VMResourceBreakdown{
				Histogram: struct {
					Data     []int `json:"data"`
					MinValue int   `json:"minValue"`
					Step     int   `json:"step"`
				}{
					Data:     []int{32, 23, 31, 14, 0, 2, 2, 1, 0, 2},
					MinValue: 0,
					Step:     38,
				},
				Total:                          7945,
				TotalForMigratableWithWarnings: 7945,
			},
			RamGB: api.VMResourceBreakdown{
				Histogram: struct {
					Data     []int `json:"data"`
					MinValue int   `json:"minValue"`
					Step     int   `json:"step"`
				}{
					Data:     []int{49, 32, 1, 14, 0, 0, 9, 0, 0, 2},
					MinValue: 1,
					Step:     5,
				},
				Total:                          1031,
				TotalForMigratableWithWarnings: 1031,
			},
			PowerStates: map[string]int{
				"poweredOff": 78,
				"poweredOn":  29,
			},
			Os: map[string]int{
				"Amazon Linux 2 (64-bit)":                1,
				"CentOS 7 (64-bit)":                      1,
				"CentOS 8 (64-bit)":                      1,
				"Debian GNU/Linux 12 (64-bit)":           1,
				"FreeBSD (64-bit)":                       2,
				"Microsoft Windows 10 (64-bit)":          2,
				"Microsoft Windows 11 (64-bit)":          2,
				"Microsoft Windows Server 2019 (64-bit)": 8,
				"Microsoft Windows Server 2022 (64-bit)": 3,
				"Microsoft Windows Server 2025 (64-bit)": 2,
				"Other (32-bit)":                         12,
				"Other (64-bit)":                         1,
				"Other 2.6.x Linux (64-bit)":             13,
				"Other Linux (64-bit)":                   1,
				"Red Hat Enterprise Linux 8 (64-bit)":    5,
				"Red Hat Enterprise Linux 9 (64-bit)":    41,
				"Red Hat Fedora (64-bit)":                2,
				"Rocky Linux (64-bit)":                   1,
				"Ubuntu Linux (64-bit)":                  3,
				"VMware ESXi 8.0 or later":               5,
			},
			MigrationWarnings: api.MigrationIssues{
				{
					Label:      "Changed Block Tracking (CBT) not enabled",
					Count:      105,
					Assessment: "Changed Block Tracking (CBT) has not been enabled on this VM. This feature is a prerequisite for VM warm migration.",
				},
				{
					Label:      "UEFI detected",
					Count:      77,
					Assessment: "UEFI secure boot will be disabled on OpenShift Virtualization. If the VM was set with UEFI secure boot, manual steps within the guest would be needed for the guest operating system to boot.",
				},
				{
					Label:      "Invalid VM Name",
					Count:      31,
					Assessment: "The VM name must comply with the DNS subdomain name format defined in RFC 1123. The name can contain lowercase letters (a-z), numbers (0-9), and hyphens (-), up to a maximum of 63 characters. The first and last characters must be alphanumeric. The name must not contain uppercase letters, spaces, periods (.), or special characters. The VM will be renamed automatically during the migration to meet the RFC convention.",
				},
				{
					Label:      "VM configured with a TPM device",
					Count:      3,
					Assessment: "The VM is configured with a TPM device. TPM data is not transferred during the migration.",
				},
				{
					Label:      "Independent disk detected",
					Count:      2,
					Assessment: "Independent disks cannot be transferred using recent versions of VDDK. It is recommended to change them in vSphere to 'Dependent' mode, or alternatively, to export the VM to an OVA.",
				},
			},
			Total:                       107,
			TotalMigratable:             107,
			TotalMigratableWithWarnings: &n,
		},
	}
}
