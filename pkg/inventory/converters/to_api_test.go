package converters

import (
	"testing"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/pkg/inventory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToAPI(t *testing.T) {
	tests := []struct {
		name     string
		input    *inventory.Inventory
		validate func(t *testing.T, result *api.Inventory)
	}{
		{
			name: "full inventory with vCenter and clusters",
			input: &inventory.Inventory{
				VCenterID: "vcenter-123",
				VCenter: &inventory.InventoryData{
					VMs: inventory.VMsData{
						Total:           10,
						TotalMigratable: 8,
						PowerStates:     map[string]int{"poweredOn": 7, "poweredOff": 3},
					},
					Infra: inventory.InfraData{
						TotalHosts: 2,
					},
				},
				Clusters: map[string]inventory.InventoryData{
					"cluster-1": {
						VMs: inventory.VMsData{
							Total:           5,
							TotalMigratable: 4,
						},
					},
					"cluster-2": {
						VMs: inventory.VMsData{
							Total:           5,
							TotalMigratable: 4,
						},
					},
				},
			},
			validate: func(t *testing.T, result *api.Inventory) {
				assert.Equal(t, "vcenter-123", result.VcenterId)
				require.NotNil(t, result.Vcenter)
				assert.Equal(t, 10, result.Vcenter.Vms.Total)
				assert.Len(t, result.Clusters, 2)
				assert.Equal(t, 5, result.Clusters["cluster-1"].Vms.Total)
				assert.Equal(t, 5, result.Clusters["cluster-2"].Vms.Total)
			},
		},
		{
			name: "nil vCenter data",
			input: &inventory.Inventory{
				VCenterID: "vcenter-456",
				VCenter:   nil,
				Clusters: map[string]inventory.InventoryData{
					"cluster-1": {
						VMs: inventory.VMsData{Total: 3},
					},
				},
			},
			validate: func(t *testing.T, result *api.Inventory) {
				assert.Equal(t, "vcenter-456", result.VcenterId)
				assert.Nil(t, result.Vcenter)
				assert.Len(t, result.Clusters, 1)
			},
		},
		{
			name: "empty clusters map",
			input: &inventory.Inventory{
				VCenterID: "vcenter-789",
				VCenter: &inventory.InventoryData{
					VMs: inventory.VMsData{Total: 5},
				},
				Clusters: map[string]inventory.InventoryData{},
			},
			validate: func(t *testing.T, result *api.Inventory) {
				assert.Equal(t, "vcenter-789", result.VcenterId)
				require.NotNil(t, result.Vcenter)
				assert.Len(t, result.Clusters, 0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ToAPI(tt.input)
			require.NotNil(t, result)
			tt.validate(t, result)
		})
	}
}

func TestToAPIVMs_ResourceBreakdowns(t *testing.T) {
	tests := []struct {
		name  string
		input inventory.VMsData
	}{
		{
			name: "all resource breakdowns populated",
			input: inventory.VMsData{
				Total:                       100,
				TotalMigratable:             80,
				TotalMigratableWithWarnings: 15,
				CPUCores: inventory.ResourceBreakdown{
					Total:                          400,
					TotalForMigratable:             320,
					TotalForMigratableWithWarnings: 60,
					TotalForNotMigratable:          20,
				},
				RamGB: inventory.ResourceBreakdown{
					Total:                          1024,
					TotalForMigratable:             800,
					TotalForMigratableWithWarnings: 150,
					TotalForNotMigratable:          74,
				},
				DiskCount: inventory.ResourceBreakdown{
					Total:                          200,
					TotalForMigratable:             160,
					TotalForMigratableWithWarnings: 30,
					TotalForNotMigratable:          10,
				},
				DiskGB: inventory.ResourceBreakdown{
					Total:                          10000,
					TotalForMigratable:             8000,
					TotalForMigratableWithWarnings: 1500,
					TotalForNotMigratable:          500,
				},
				NicCount: inventory.ResourceBreakdown{
					Total:                          150,
					TotalForMigratable:             120,
					TotalForMigratableWithWarnings: 20,
					TotalForNotMigratable:          10,
				},
			},
		},
		{
			name: "zero values handled",
			input: inventory.VMsData{
				Total: 0,
				CPUCores: inventory.ResourceBreakdown{
					Total:                          0,
					TotalForMigratable:             0,
					TotalForMigratableWithWarnings: 0,
					TotalForNotMigratable:          0,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toAPIVMs(&tt.input)

			// Verify CPU cores breakdown
			assert.Equal(t, tt.input.CPUCores.Total, result.CpuCores.Total)
			assert.Equal(t, tt.input.CPUCores.TotalForMigratable, result.CpuCores.TotalForMigratable)
			assert.Equal(t, tt.input.CPUCores.TotalForMigratableWithWarnings, result.CpuCores.TotalForMigratableWithWarnings)
			assert.Equal(t, tt.input.CPUCores.TotalForNotMigratable, result.CpuCores.TotalForNotMigratable)

			// Verify RAM breakdown
			assert.Equal(t, tt.input.RamGB.Total, result.RamGB.Total)
			assert.Equal(t, tt.input.RamGB.TotalForMigratable, result.RamGB.TotalForMigratable)

			// Verify disk count breakdown
			assert.Equal(t, tt.input.DiskCount.Total, result.DiskCount.Total)

			// Verify disk GB breakdown
			assert.Equal(t, tt.input.DiskGB.Total, result.DiskGB.Total)

			// Verify NIC count breakdown
			require.NotNil(t, result.NicCount)
			assert.Equal(t, tt.input.NicCount.Total, result.NicCount.Total)
		})
	}
}

func TestToAPIVMs_OSInfo(t *testing.T) {
	tests := []struct {
		name  string
		input inventory.VMsData
	}{
		{
			name: "OS with upgrade recommendation",
			input: inventory.VMsData{
				OSInfo: map[string]inventory.OSInfo{
					"Red Hat Enterprise Linux 7": {
						Count:                 10,
						IsSupported:           true,
						UpgradeRecommendation: "Consider upgrading to RHEL 8 or 9",
					},
				},
			},
		},
		{
			name: "OS without upgrade recommendation",
			input: inventory.VMsData{
				OSInfo: map[string]inventory.OSInfo{
					"Red Hat Enterprise Linux 9": {
						Count:                 5,
						IsSupported:           true,
						UpgradeRecommendation: "",
					},
				},
			},
		},
		{
			name: "empty OS info map",
			input: inventory.VMsData{
				OSInfo: map[string]inventory.OSInfo{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toAPIVMs(&tt.input)

			require.NotNil(t, result.OsInfo)
			osInfo := *result.OsInfo

			for name, expected := range tt.input.OSInfo {
				actual, exists := osInfo[name]
				require.True(t, exists, "OS %s should exist in result", name)
				assert.Equal(t, expected.Count, actual.Count)
				assert.Equal(t, expected.IsSupported, actual.Supported)

				if expected.UpgradeRecommendation != "" {
					require.NotNil(t, actual.UpgradeRecommendation)
					assert.Equal(t, expected.UpgradeRecommendation, *actual.UpgradeRecommendation)
				} else {
					assert.Nil(t, actual.UpgradeRecommendation)
				}
			}
		})
	}
}

func TestToAPIVMs_MigrationIssues(t *testing.T) {
	tests := []struct {
		name  string
		input inventory.VMsData
	}{
		{
			name: "multiple warnings and criticals",
			input: inventory.VMsData{
				MigrationWarnings: []inventory.MigrationIssue{
					{ID: "cbt.disabled", Label: "CBT disabled", Assessment: "Consider enabling CBT", Count: 5},
					{ID: "numa.affinity", Label: "NUMA affinity", Assessment: "May affect performance", Count: 3},
				},
				NotMigratableReasons: []inventory.MigrationIssue{
					{ID: "template.vm", Label: "Template VM", Assessment: "Templates cannot be migrated", Count: 2},
				},
			},
		},
		{
			name: "empty issues",
			input: inventory.VMsData{
				MigrationWarnings:    []inventory.MigrationIssue{},
				NotMigratableReasons: []inventory.MigrationIssue{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toAPIVMs(&tt.input)

			assert.Len(t, result.MigrationWarnings, len(tt.input.MigrationWarnings))
			assert.Len(t, result.NotMigratableReasons, len(tt.input.NotMigratableReasons))

			for i, expected := range tt.input.MigrationWarnings {
				actual := result.MigrationWarnings[i]
				require.NotNil(t, actual.Id)
				assert.Equal(t, expected.ID, *actual.Id)
				assert.Equal(t, expected.Label, actual.Label)
				assert.Equal(t, expected.Assessment, actual.Assessment)
				assert.Equal(t, expected.Count, actual.Count)
			}

			for i, expected := range tt.input.NotMigratableReasons {
				actual := result.NotMigratableReasons[i]
				require.NotNil(t, actual.Id)
				assert.Equal(t, expected.ID, *actual.Id)
			}
		})
	}
}

func TestAnonymizeNFSDatastore(t *testing.T) {
	tests := []struct {
		name             string
		datastoreType    string
		originalDiskId   string
		originalProtocol string
		expectAnonymized bool
	}{
		{
			name:             "NFS lowercase - should anonymize",
			datastoreType:    "nfs",
			originalDiskId:   "nfs://server/path",
			originalProtocol: "NFS v3",
			expectAnonymized: true,
		},
		{
			name:             "NFS uppercase - should anonymize",
			datastoreType:    "NFS",
			originalDiskId:   "nfs://server/path",
			originalProtocol: "NFS v4",
			expectAnonymized: true,
		},
		{
			name:             "NFS mixed case - should anonymize",
			datastoreType:    "Nfs",
			originalDiskId:   "nfs://server/path",
			originalProtocol: "NFS",
			expectAnonymized: true,
		},
		{
			name:             "VMFS - should NOT anonymize",
			datastoreType:    "VMFS",
			originalDiskId:   "vmfs-12345",
			originalProtocol: "SCSI",
			expectAnonymized: false,
		},
		{
			name:             "VSAN - should NOT anonymize",
			datastoreType:    "VSAN",
			originalDiskId:   "vsan-uuid",
			originalProtocol: "VSAN",
			expectAnonymized: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := &api.Datastore{
				Type:         tt.datastoreType,
				DiskId:       tt.originalDiskId,
				ProtocolType: tt.originalProtocol,
			}

			anonymizeNFSDatastore(ds)

			if tt.expectAnonymized {
				assert.Equal(t, "N/A", ds.DiskId, "DiskId should be anonymized for NFS")
				assert.Equal(t, "N/A", ds.ProtocolType, "ProtocolType should be anonymized for NFS")
			} else {
				assert.Equal(t, tt.originalDiskId, ds.DiskId, "DiskId should NOT be changed for non-NFS")
				assert.Equal(t, tt.originalProtocol, ds.ProtocolType, "ProtocolType should NOT be changed for non-NFS")
			}
		})
	}
}

func TestToAPIInfra_Hosts(t *testing.T) {
	tests := []struct {
		name  string
		input inventory.InfraData
	}{
		{
			name: "host with all fields populated",
			input: inventory.InfraData{
				Hosts: []inventory.Host{
					{
						ID:         "host-123",
						Vendor:     "Dell",
						Model:      "PowerEdge R740",
						CpuCores:   32,
						CpuSockets: 2,
						MemoryMB:   131072,
					},
				},
				HostPowerStates: map[string]int{"poweredOn": 1},
				TotalHosts:      1,
			},
		},
		{
			name: "host with zero values - pointers should be nil",
			input: inventory.InfraData{
				Hosts: []inventory.Host{
					{
						ID:         "",
						Vendor:     "VMware",
						Model:      "ESXi",
						CpuCores:   0,
						CpuSockets: 0,
						MemoryMB:   0,
					},
				},
				TotalHosts: 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toAPIInfra(&tt.input)

			require.NotNil(t, result.Hosts)
			hosts := *result.Hosts
			require.Len(t, hosts, len(tt.input.Hosts))

			for i, expected := range tt.input.Hosts {
				actual := hosts[i]
				assert.Equal(t, expected.Vendor, actual.Vendor)
				assert.Equal(t, expected.Model, actual.Model)

				// Check optional pointer fields
				if expected.ID != "" {
					require.NotNil(t, actual.Id)
					assert.Equal(t, expected.ID, *actual.Id)
				} else {
					assert.Nil(t, actual.Id)
				}

				if expected.CpuCores > 0 {
					require.NotNil(t, actual.CpuCores)
					assert.Equal(t, expected.CpuCores, *actual.CpuCores)
				} else {
					assert.Nil(t, actual.CpuCores)
				}

				if expected.CpuSockets > 0 {
					require.NotNil(t, actual.CpuSockets)
					assert.Equal(t, expected.CpuSockets, *actual.CpuSockets)
				} else {
					assert.Nil(t, actual.CpuSockets)
				}

				if expected.MemoryMB > 0 {
					require.NotNil(t, actual.MemoryMB)
					assert.Equal(t, int64(expected.MemoryMB), *actual.MemoryMB)
				} else {
					assert.Nil(t, actual.MemoryMB)
				}
			}
		})
	}
}

func TestToAPIInfra_Networks(t *testing.T) {
	tests := []struct {
		name  string
		input inventory.InfraData
	}{
		{
			name: "network with optional fields",
			input: inventory.InfraData{
				Networks: []inventory.Network{
					{
						Name:     "VM Network",
						Type:     "dvportgroup",
						Dvswitch: "dvSwitch1",
						VlanId:   "100",
						VmsCount: 25,
					},
				},
			},
		},
		{
			name: "network without optional fields",
			input: inventory.InfraData{
				Networks: []inventory.Network{
					{
						Name:     "Management Network",
						Type:     "standard",
						Dvswitch: "",
						VlanId:   "",
						VmsCount: 0,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toAPIInfra(&tt.input)

			require.Len(t, result.Networks, len(tt.input.Networks))

			for i, expected := range tt.input.Networks {
				actual := result.Networks[i]
				assert.Equal(t, expected.Name, actual.Name)
				assert.Equal(t, api.NetworkType(expected.Type), actual.Type)

				if expected.Dvswitch != "" {
					require.NotNil(t, actual.Dvswitch)
					assert.Equal(t, expected.Dvswitch, *actual.Dvswitch)
				} else {
					assert.Nil(t, actual.Dvswitch)
				}

				if expected.VlanId != "" {
					require.NotNil(t, actual.VlanId)
					assert.Equal(t, expected.VlanId, *actual.VlanId)
				} else {
					assert.Nil(t, actual.VlanId)
				}

				if expected.VmsCount > 0 {
					require.NotNil(t, actual.VmsCount)
					assert.Equal(t, expected.VmsCount, *actual.VmsCount)
				} else {
					assert.Nil(t, actual.VmsCount)
				}
			}
		})
	}
}

func TestToAPIInfra_Datastores(t *testing.T) {
	tests := []struct {
		name  string
		input inventory.InfraData
	}{
		{
			name: "VMFS datastore with host",
			input: inventory.InfraData{
				Datastores: []inventory.Datastore{
					{
						DiskId:          "disk-123",
						FreeCapacityGB:  500.5,
						TotalCapacityGB: 1000.0,
						Type:            "VMFS",
						HostId:          "host-1",
						Model:           "SSD",
						ProtocolType:    "SCSI",
						Vendor:          "Intel",
					},
				},
			},
		},
		{
			name: "NFS datastore - should be anonymized",
			input: inventory.InfraData{
				Datastores: []inventory.Datastore{
					{
						DiskId:          "nfs://server/path",
						FreeCapacityGB:  200.0,
						TotalCapacityGB: 500.0,
						Type:            "NFS",
						HostId:          "",
						Model:           "",
						ProtocolType:    "NFS v3",
						Vendor:          "",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toAPIInfra(&tt.input)

			require.Len(t, result.Datastores, len(tt.input.Datastores))

			for i, expected := range tt.input.Datastores {
				actual := result.Datastores[i]

				// Check NFS anonymization
				if expected.Type == "NFS" || expected.Type == "nfs" {
					assert.Equal(t, "N/A", actual.DiskId, "NFS DiskId should be anonymized")
					assert.Equal(t, "N/A", actual.ProtocolType, "NFS ProtocolType should be anonymized")
				} else {
					assert.Equal(t, expected.DiskId, actual.DiskId)
					assert.Equal(t, expected.ProtocolType, actual.ProtocolType)
				}

				assert.Equal(t, int(expected.FreeCapacityGB), actual.FreeCapacityGB)
				assert.Equal(t, int(expected.TotalCapacityGB), actual.TotalCapacityGB)
				assert.Equal(t, expected.Type, actual.Type)

				if expected.HostId != "" {
					require.NotNil(t, actual.HostId)
					assert.Equal(t, expected.HostId, *actual.HostId)
				} else {
					assert.Nil(t, actual.HostId)
				}
			}
		})
	}
}

func TestToAPIInfra_DatacenterInfo(t *testing.T) {
	tests := []struct {
		name                string
		input               inventory.InfraData
		expectDatacenters   bool
		expectClustersPerDC bool
	}{
		{
			name: "with datacenter info",
			input: inventory.InfraData{
				TotalDatacenters:      3,
				ClustersPerDatacenter: []int{2, 3, 1},
			},
			expectDatacenters:   true,
			expectClustersPerDC: true,
		},
		{
			name: "zero datacenters - pointer should be nil",
			input: inventory.InfraData{
				TotalDatacenters:      0,
				ClustersPerDatacenter: nil,
			},
			expectDatacenters:   false,
			expectClustersPerDC: false,
		},
		{
			name: "empty clusters per DC",
			input: inventory.InfraData{
				TotalDatacenters:      2,
				ClustersPerDatacenter: []int{},
			},
			expectDatacenters:   true,
			expectClustersPerDC: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toAPIInfra(&tt.input)

			if tt.expectDatacenters {
				require.NotNil(t, result.TotalDatacenters)
				assert.Equal(t, tt.input.TotalDatacenters, *result.TotalDatacenters)
			} else {
				assert.Nil(t, result.TotalDatacenters)
			}

			if tt.expectClustersPerDC {
				require.NotNil(t, result.ClustersPerDatacenter)
				assert.Equal(t, tt.input.ClustersPerDatacenter, *result.ClustersPerDatacenter)
			} else {
				assert.Nil(t, result.ClustersPerDatacenter)
			}
		})
	}
}

func TestToAPIInfra_Overcommitment(t *testing.T) {
	cpuRatio := 2.5
	memRatio := 1.5

	tests := []struct {
		name  string
		input inventory.InfraData
	}{
		{
			name: "with overcommitment values",
			input: inventory.InfraData{
				CPUOverCommitment:    &cpuRatio,
				MemoryOverCommitment: &memRatio,
			},
		},
		{
			name: "nil overcommitment values",
			input: inventory.InfraData{
				CPUOverCommitment:    nil,
				MemoryOverCommitment: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toAPIInfra(&tt.input)

			if tt.input.CPUOverCommitment != nil {
				require.NotNil(t, result.CpuOverCommitment)
				assert.Equal(t, *tt.input.CPUOverCommitment, *result.CpuOverCommitment)
			} else {
				assert.Nil(t, result.CpuOverCommitment)
			}

			if tt.input.MemoryOverCommitment != nil {
				require.NotNil(t, result.MemoryOverCommitment)
				assert.Equal(t, *tt.input.MemoryOverCommitment, *result.MemoryOverCommitment)
			} else {
				assert.Nil(t, result.MemoryOverCommitment)
			}
		})
	}
}

func TestToAPIVMs_Distributions(t *testing.T) {
	input := inventory.VMsData{
		DistributionByCPUTier:    map[string]int{"1-2": 10, "3-4": 20, "5+": 5},
		DistributionByMemoryTier: map[string]int{"0-4GB": 5, "4-8GB": 15, "8GB+": 15},
		DistributionByNICCount:   map[string]int{"1": 25, "2": 8, "3+": 2},
		DiskSizeTiers: map[string]inventory.DiskSizeTierSummary{
			"0-100GB":   {VMCount: 10, TotalSizeTB: 0.5},
			"100-500GB": {VMCount: 15, TotalSizeTB: 3.0},
		},
		DiskTypes: map[string]inventory.DiskTypeSummary{
			"VMFS": {Type: "VMFS", VMCount: 20, TotalSizeTB: 5.0},
			"NFS":  {Type: "NFS", VMCount: 5, TotalSizeTB: 1.0},
		},
	}

	result := toAPIVMs(&input)

	// Verify CPU tier distribution
	require.NotNil(t, result.DistributionByCpuTier)
	assert.Equal(t, input.DistributionByCPUTier, *result.DistributionByCpuTier)

	// Verify memory tier distribution
	require.NotNil(t, result.DistributionByMemoryTier)
	assert.Equal(t, input.DistributionByMemoryTier, *result.DistributionByMemoryTier)

	// Verify NIC count distribution
	require.NotNil(t, result.DistributionByNicCount)
	assert.Equal(t, input.DistributionByNICCount, *result.DistributionByNicCount)

	// Verify disk size tiers
	require.NotNil(t, result.DiskSizeTier)
	for tier, expected := range input.DiskSizeTiers {
		actual, exists := (*result.DiskSizeTier)[tier]
		require.True(t, exists)
		assert.Equal(t, expected.VMCount, actual.VmCount)
		assert.Equal(t, expected.TotalSizeTB, actual.TotalSizeTB)
	}

	// Verify disk types
	require.NotNil(t, result.DiskTypes)
	for diskType, expected := range input.DiskTypes {
		actual, exists := (*result.DiskTypes)[diskType]
		require.True(t, exists)
		assert.Equal(t, expected.VMCount, actual.VmCount)
		assert.Equal(t, expected.TotalSizeTB, actual.TotalSizeTB)
	}
}
