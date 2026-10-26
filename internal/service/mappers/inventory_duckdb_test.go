package mappers

import (
	"encoding/json"
	"testing"
	"time"

	svcmodel "github.com/kubev2v/migration-planner/internal/model"
	"github.com/kubev2v/migration-planner/pkg/inventory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInventoryFromDuckDB(t *testing.T) {
	tests := []struct {
		name     string
		input    *inventory.Inventory
		validate func(t *testing.T, result *svcmodel.Inventory)
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
			validate: func(t *testing.T, result *svcmodel.Inventory) {
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
			validate: func(t *testing.T, result *svcmodel.Inventory) {
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
			validate: func(t *testing.T, result *svcmodel.Inventory) {
				assert.Equal(t, "vcenter-789", result.VcenterId)
				require.NotNil(t, result.Vcenter)
				assert.Len(t, result.Clusters, 0)
			},
		},
		{
			name: "with createdAt set",
			input: &inventory.Inventory{
				VCenterID: "vcenter-time",
				Clusters:  map[string]inventory.InventoryData{},
				CreatedAt: timePtr(time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)),
			},
			validate: func(t *testing.T, result *svcmodel.Inventory) {
				require.NotNil(t, result.CreatedAt)
				assert.Equal(t, time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC), *result.CreatedAt)
			},
		},
		{
			name: "without createdAt",
			input: &inventory.Inventory{
				VCenterID: "vcenter-no-time",
				Clusters:  map[string]inventory.InventoryData{},
			},
			validate: func(t *testing.T, result *svcmodel.Inventory) {
				assert.Nil(t, result.CreatedAt)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := InventoryFromDuckDB(tt.input)
			require.NotNil(t, result)
			tt.validate(t, result)
		})
	}
}

func TestVMsFromDuckDB_ResourceBreakdowns(t *testing.T) {
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
				TotalWithSharedDisks:        12,
				TotalWithRDM:                7,
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
			result := vmsFromDuckDB(&tt.input)

			// Verify shared-disks / RDM aggregate counts
			require.NotNil(t, result.TotalWithSharedDisks)
			assert.Equal(t, tt.input.TotalWithSharedDisks, *result.TotalWithSharedDisks)
			require.NotNil(t, result.TotalWithRDM)
			assert.Equal(t, tt.input.TotalWithRDM, *result.TotalWithRDM)

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

func TestVMsFromDuckDB_OSInfo(t *testing.T) {
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
						SupportTier:           inventory.TierCertified,
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
						SupportTier:           inventory.TierCertified,
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
			result := vmsFromDuckDB(&tt.input)

			require.NotNil(t, result.OsInfo)
			osInfo := *result.OsInfo

			for name, expected := range tt.input.OSInfo {
				actual, exists := osInfo[name]
				require.True(t, exists, "OS %s should exist in result", name)
				assert.Equal(t, expected.Count, actual.Count)
				assert.Equal(t, expected.IsSupported, actual.Supported)
				expectedTier := svcmodel.OsInfoSupportTier(expected.SupportTier)
				assert.Equal(t, &expectedTier, actual.SupportTier)

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

func TestVMsFromDuckDB_MigrationIssues(t *testing.T) {
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
			result := vmsFromDuckDB(&tt.input)

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
		name                 string
		datastoreType        string
		originalDiskId       string
		originalProtocol     string
		expectDiskIdRedacted bool
	}{
		{
			name:                 "NFS with address and export path - redacts diskId",
			datastoreType:        "NFS",
			originalDiskId:       "10.46.29.166 /nfs-data/exports",
			originalProtocol:     "NFS",
			expectDiskIdRedacted: true,
		},
		{
			name:                 "NFS lowercase with server and volume - redacts diskId",
			datastoreType:        "nfs",
			originalDiskId:       "server:/export/share",
			originalProtocol:     "NFS v3",
			expectDiskIdRedacted: true,
		},
		{
			name:                 "NFS with bare IPv4 address - redacts diskId",
			datastoreType:        "NFS",
			originalDiskId:       "10.46.29.166",
			originalProtocol:     "NFS",
			expectDiskIdRedacted: true,
		},
		{
			name:                 "NFS with plain datastore name - keeps diskId",
			datastoreType:        "Nfs",
			originalDiskId:       "netappnfs",
			originalProtocol:     "NFS41",
			expectDiskIdRedacted: false,
		},
		{
			name:                 "NFS with name containing dots - keeps diskId",
			datastoreType:        "NFS",
			originalDiskId:       "ds.prod.nfs",
			originalProtocol:     "NFS",
			expectDiskIdRedacted: false,
		},
		{
			name:                 "VMFS - should NOT anonymize",
			datastoreType:        "VMFS",
			originalDiskId:       "vmfs-12345",
			originalProtocol:     "SCSI",
			expectDiskIdRedacted: false,
		},
		{
			name:                 "VSAN - should NOT anonymize",
			datastoreType:        "VSAN",
			originalDiskId:       "vsan-uuid",
			originalProtocol:     "VSAN",
			expectDiskIdRedacted: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := &svcmodel.Datastore{
				Type:         tt.datastoreType,
				DiskId:       tt.originalDiskId,
				ProtocolType: tt.originalProtocol,
			}

			anonymizeNFSDatastore(ds)

			// The protocol label is never sensitive and must survive anonymization.
			assert.Equal(t, tt.originalProtocol, ds.ProtocolType, "ProtocolType should never be anonymized")
			if tt.expectDiskIdRedacted {
				assert.Equal(t, "N/A", ds.DiskId, "DiskId should be redacted for address-like NFS identifiers")
			} else {
				assert.Equal(t, tt.originalDiskId, ds.DiskId, "DiskId should be kept for plain datastore names")
			}
		})
	}
}

func TestVMsFromDuckDB_DiskComplexityTier(t *testing.T) {
	tests := []struct {
		name           string
		input          inventory.VMsData
		expectNilField bool
	}{
		{
			name: "nil DiskComplexityTiers produces nil service field",
			input: inventory.VMsData{
				DiskComplexityTiers: nil,
			},
			expectNilField: true,
		},
		{
			name: "empty DiskComplexityTiers produces nil service field",
			input: inventory.VMsData{
				DiskComplexityTiers: map[string]inventory.DiskSizeTierSummary{},
			},
			expectNilField: true,
		},
		{
			name: "populated DiskComplexityTiers produces non-nil service field with correct values",
			input: inventory.VMsData{
				DiskComplexityTiers: map[string]inventory.DiskSizeTierSummary{
					"0-10TiB":  {VMCount: 50, TotalSizeTB: 3.5},
					"10-20TiB": {VMCount: 10, TotalSizeTB: 12.0},
				},
			},
			expectNilField: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := vmsFromDuckDB(&tt.input)

			if tt.expectNilField {
				assert.Nil(t, result.DiskComplexityTier,
					"DiskComplexityTier should be nil so omitempty omits it from JSON")
			} else {
				require.NotNil(t, result.DiskComplexityTier)
				for tier, expected := range tt.input.DiskComplexityTiers {
					actual, exists := (*result.DiskComplexityTier)[tier]
					require.True(t, exists, "tier %s should exist in result", tier)
					assert.Equal(t, expected.VMCount, actual.VmCount)
					assert.Equal(t, expected.TotalSizeTB, actual.TotalSizeTB)
				}
			}
		})
	}
}

func TestInfraFromDuckDB_Hosts(t *testing.T) {
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
			result := infraFromDuckDB(&tt.input)

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

func TestInfraFromDuckDB_Networks(t *testing.T) {
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
			result := infraFromDuckDB(&tt.input)

			require.Len(t, result.Networks, len(tt.input.Networks))

			for i, expected := range tt.input.Networks {
				actual := result.Networks[i]
				assert.Equal(t, expected.Name, actual.Name)
				assert.Equal(t, svcmodel.NetworkType(expected.Type), actual.Type)

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

func TestInfraFromDuckDB_Datastores(t *testing.T) {
	tests := []struct {
		name                string
		input               inventory.InfraData
		wantStorageProtocol svcmodel.DatastoreStorageProtocol
		wantStorageMetadata string
	}{
		{
			name:                "VMFS datastore with host",
			wantStorageProtocol: svcmodel.DatastoreStorageProtocol("Block"),
			wantStorageMetadata: `{"filesystemType":"VMFS","filesystemMajorVersion":6,"backingDeviceIds":["naa.123","naa.456"]}`,
			input: inventory.InfraData{
				Datastores: []inventory.Datastore{
					{
						DiskId:                 "disk-123",
						FilesystemType:         "VMFS",
						FilesystemMajorVersion: 6,
						BackingDeviceIDs:       []string{"naa.123", "naa.456"},
						FreeCapacityGB:         500.5,
						TotalCapacityGB:        1000.0,
						Type:                   "VMFS",
						StorageProtocol:        inventory.StorageProtocolBlock,
						VsanSnapshotSupport:    true,
						HostId:                 "host-1",
						Model:                  "SSD",
						ProtocolType:           "SCSI",
						Vendor:                 "Intel",
					},
				},
			},
		},
		{
			name:                "NFS datastore with address-like diskId - should be redacted",
			wantStorageProtocol: svcmodel.DatastoreStorageProtocol("File"),
			input: inventory.InfraData{
				Datastores: []inventory.Datastore{
					{
						DiskId:          "nfs://server/path",
						FreeCapacityGB:  200.0,
						TotalCapacityGB: 500.0,
						Type:            "NFS",
						StorageProtocol: inventory.StorageProtocolFile,
						HostId:          "",
						Model:           "",
						ProtocolType:    "NFS v3",
						Vendor:          "",
					},
				},
			},
		},
		{
			name:                "NFS datastore with plain name - should be kept",
			wantStorageProtocol: svcmodel.DatastoreStorageProtocol("File"),
			input: inventory.InfraData{
				Datastores: []inventory.Datastore{
					{
						DiskId:          "netappnfs",
						FreeCapacityGB:  200.0,
						TotalCapacityGB: 500.0,
						Type:            "NFS",
						StorageProtocol: inventory.StorageProtocolFile,
						HostId:          "host-1",
						Model:           "",
						ProtocolType:    "NFS",
						Vendor:          "",
					},
				},
			},
		},
		{
			name: "unknown datastore type - storage protocol should be omitted",
			input: inventory.InfraData{
				Datastores: []inventory.Datastore{
					{
						DiskId:          "disk-unknown",
						FreeCapacityGB:  100.0,
						TotalCapacityGB: 200.0,
						Type:            "OTHER",
						ProtocolType:    "unknown",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := infraFromDuckDB(&tt.input)

			require.Len(t, result.Datastores, len(tt.input.Datastores))

			for i, expected := range tt.input.Datastores {
				actual := result.Datastores[i]

				data, err := json.Marshal(actual)
				require.NoError(t, err)
				var fields map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(data, &fields))
				metadata := make(map[string]json.RawMessage)
				for _, key := range []string{"filesystemType", "filesystemMajorVersion", "backingDeviceIds"} {
					if value, ok := fields[key]; ok {
						metadata[key] = value
					}
				}
				encodedMetadata, err := json.Marshal(metadata)
				require.NoError(t, err)
				wantMetadata := tt.wantStorageMetadata
				if wantMetadata == "" {
					wantMetadata = "{}"
				}
				assert.JSONEq(t, wantMetadata, string(encodedMetadata))

				// Check NFS redaction: address-like identifiers are redacted, the
				// protocol label is never redacted.
				if expected.Type == "NFS" || expected.Type == "nfs" {
					assert.Equal(t, expected.ProtocolType, actual.ProtocolType, "NFS ProtocolType should never be anonymized")
					if nfsDiskIdIsAddressLike(expected.DiskId) {
						assert.Equal(t, "N/A", actual.DiskId, "address-like NFS DiskId should be redacted")
					} else {
						assert.Equal(t, expected.DiskId, actual.DiskId, "plain NFS DiskId should be kept")
					}
				} else {
					assert.Equal(t, expected.DiskId, actual.DiskId)
					assert.Equal(t, expected.ProtocolType, actual.ProtocolType)
				}

				assert.Equal(t, int(expected.FreeCapacityGB), actual.FreeCapacityGB)
				assert.Equal(t, int(expected.TotalCapacityGB), actual.TotalCapacityGB)
				assert.Equal(t, expected.Type, actual.Type)
				if tt.wantStorageProtocol != "" {
					require.NotNil(t, actual.StorageProtocol)
					assert.Equal(t, tt.wantStorageProtocol, *actual.StorageProtocol)
				} else {
					assert.Nil(t, actual.StorageProtocol)
				}

				require.NotNil(t, actual.VsanSnapshotSupport)
				assert.Equal(t, expected.VsanSnapshotSupport, *actual.VsanSnapshotSupport)
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

func TestInfraFromDuckDB_DatacenterInfo(t *testing.T) {
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
			result := infraFromDuckDB(&tt.input)

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

func TestInfraFromDuckDB_Overcommitment(t *testing.T) {
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
			result := infraFromDuckDB(&tt.input)

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

func TestInventoryFromDuckDB_VMsPerHostAverage(t *testing.T) {
	tests := []struct {
		name       string
		totalVMs   int
		totalHosts int
		want       *float64
	}{
		{name: "10 VMs 2 hosts", totalVMs: 10, totalHosts: 2, want: floatPtr(5.0)},
		{name: "328 VMs 7 hosts", totalVMs: 328, totalHosts: 7, want: floatPtr(46.86)},
		{name: "0 hosts", totalVMs: 10, totalHosts: 0, want: nil},
		{name: "201 VMs 200 hosts half-cent rounds up", totalVMs: 201, totalHosts: 200, want: floatPtr(1.01)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := InventoryFromDuckDB(&inventory.Inventory{
				VCenter: &inventory.InventoryData{
					VMs:   inventory.VMsData{Total: tt.totalVMs},
					Infra: inventory.InfraData{TotalHosts: tt.totalHosts},
				},
			})
			require.NotNil(t, result.Vcenter)
			if tt.want == nil {
				assert.Nil(t, result.Vcenter.Infra.VmsPerHostAverage)
				return
			}
			require.NotNil(t, result.Vcenter.Infra.VmsPerHostAverage)
			assert.Equal(t, *tt.want, *result.Vcenter.Infra.VmsPerHostAverage)
		})
	}
}

func TestInventoryFromDuckDB_TotalWithFaultTolerance(t *testing.T) {
	result := InventoryFromDuckDB(&inventory.Inventory{
		VCenter: &inventory.InventoryData{
			VMs: inventory.VMsData{Total: 10, TotalWithFaultTolerance: 3},
		},
	})
	require.NotNil(t, result.Vcenter)
	require.NotNil(t, result.Vcenter.Vms.TotalWithFaultTolerance)
	assert.Equal(t, 3, *result.Vcenter.Vms.TotalWithFaultTolerance)
}

func TestInfraFromDuckDB_StandaloneHostsDetected(t *testing.T) {
	tests := []struct {
		name  string
		input bool
	}{
		{name: "standalone hosts detected", input: true},
		{name: "no standalone hosts", input: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := infraFromDuckDB(&inventory.InfraData{StandaloneHostsDetected: tt.input})
			require.NotNil(t, result.StandaloneHostsDetected)
			assert.Equal(t, tt.input, *result.StandaloneHostsDetected)
		})
	}
}

func TestVMsFromDuckDB_Distributions(t *testing.T) {
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
		DistributionByComplexity: map[string]int{"1": 5, "2": 3},
		ComplexityDistribution: map[string]inventory.DiskSizeTierSummary{
			"1": {VMCount: 5, TotalSizeTB: 0.05},
			"2": {VMCount: 3, TotalSizeTB: 1.5},
		},
	}

	result := vmsFromDuckDB(&input)

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

	// Verify old integer field is passed through directly
	require.NotNil(t, result.DistributionByComplexity)
	assert.Equal(t, input.DistributionByComplexity, *result.DistributionByComplexity)

	// Verify new enriched field
	require.NotNil(t, result.ComplexityDistribution)
	for level, expected := range input.ComplexityDistribution {
		actual, exists := (*result.ComplexityDistribution)[level]
		require.True(t, exists, "complexity level %s should exist in result", level)
		assert.Equal(t, expected.VMCount, actual.VmCount)
		assert.Equal(t, expected.TotalSizeTB, actual.TotalSizeTB)
	}
}

func TestInventoryFromDuckDB_ClusterUtilization(t *testing.T) {
	t.Run("with cluster utilization data", func(t *testing.T) {
		domainInv := &inventory.Inventory{
			VCenterID: "test-vcenter",
			Clusters: map[string]inventory.InventoryData{
				"cluster-1": {
					VMs:   inventory.VMsData{Total: 10},
					Infra: inventory.InfraData{TotalHosts: 3},
					ClusterUtilization: &inventory.ClusterUtilization{
						CpuAvg:     35.5,
						CpuP95:     45.2,
						CpuMax:     78.3,
						MemAvg:     52.1,
						MemP95:     62.3,
						MemMax:     85.7,
						Confidence: 87.5,
					},
				},
			},
		}

		serviceInv := InventoryFromDuckDB(domainInv)

		require.Contains(t, serviceInv.Clusters, "cluster-1")
		clusterData := serviceInv.Clusters["cluster-1"]
		require.NotNil(t, clusterData.ClusterUtilization)

		util := clusterData.ClusterUtilization
		assert.Equal(t, 35.5, util.CpuAvg)
		assert.Equal(t, 45.2, util.CpuP95)
		assert.Equal(t, 78.3, util.CpuMax)
		assert.Equal(t, 52.1, util.MemAvg)
		assert.Equal(t, 62.3, util.MemP95)
		assert.Equal(t, 85.7, util.MemMax)
		assert.Equal(t, 87.5, util.Confidence)
	})

	t.Run("without cluster utilization data", func(t *testing.T) {
		domainInv := &inventory.Inventory{
			VCenterID: "test-vcenter",
			Clusters: map[string]inventory.InventoryData{
				"cluster-1": {
					VMs:   inventory.VMsData{Total: 10},
					Infra: inventory.InfraData{TotalHosts: 3},
				},
			},
		}

		serviceInv := InventoryFromDuckDB(domainInv)

		require.Contains(t, serviceInv.Clusters, "cluster-1")
		assert.Nil(t, serviceInv.Clusters["cluster-1"].ClusterUtilization)
	})

	t.Run("with multiple clusters", func(t *testing.T) {
		domainInv := &inventory.Inventory{
			VCenterID: "test-vcenter",
			Clusters: map[string]inventory.InventoryData{
				"cluster-1": {
					VMs: inventory.VMsData{Total: 10},
					ClusterUtilization: &inventory.ClusterUtilization{
						CpuP95:     45.2,
						MemP95:     62.3,
						Confidence: 87.5,
					},
				},
				"cluster-2": {
					VMs: inventory.VMsData{Total: 20},
					ClusterUtilization: &inventory.ClusterUtilization{
						CpuP95:     38.7,
						MemP95:     55.1,
						Confidence: 92.0,
					},
				},
			},
		}

		serviceInv := InventoryFromDuckDB(domainInv)

		assert.Len(t, serviceInv.Clusters, 2)
		assert.NotNil(t, serviceInv.Clusters["cluster-1"].ClusterUtilization)
		assert.NotNil(t, serviceInv.Clusters["cluster-2"].ClusterUtilization)
		assert.Equal(t, 45.2, serviceInv.Clusters["cluster-1"].ClusterUtilization.CpuP95)
		assert.Equal(t, 38.7, serviceInv.Clusters["cluster-2"].ClusterUtilization.CpuP95)
	})
}

func timePtr(t time.Time) *time.Time { return &t }

func floatPtr(v float64) *float64 { return &v }

func TestInventoryFromDuckDB_CollectionTime(t *testing.T) {
	collectionTime := time.Date(2026, 9, 20, 10, 30, 0, 0, time.UTC)
	createdAt := collectionTime.Add(24 * time.Hour)
	for _, value := range []*time.Time{nil, &collectionTime} {
		converted := InventoryFromDuckDB(&inventory.Inventory{CreatedAt: &createdAt, CollectionTime: value})
		data, err := json.Marshal(converted)
		require.NoError(t, err)
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(data, &fields))
		require.Contains(t, fields, "collection_time")
		if value == nil {
			require.JSONEq(t, "null", string(fields["collection_time"]))
		}
		var roundTrip svcmodel.Inventory
		require.NoError(t, json.Unmarshal(data, &roundTrip))
		require.Equal(t, value, roundTrip.CollectionTime)
		require.Equal(t, &createdAt, roundTrip.CreatedAt)
	}
	var older svcmodel.Inventory
	require.NoError(t, json.Unmarshal([]byte(`{"vcenter_id":"vc-1","clusters":{}}`), &older))
	require.Nil(t, older.CollectionTime)
}
