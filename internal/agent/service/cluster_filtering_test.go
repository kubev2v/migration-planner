package service

import (
	"testing"

	vsphere "github.com/kubev2v/forklift/pkg/controller/provider/model/vsphere"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/stretchr/testify/assert"
)

func TestFilterDatastoresByVMs(t *testing.T) {
	tests := []struct {
		name                 string
		datastores           []api.Datastore
		vms                  []vsphere.VM
		datastoreMapping     map[string]string
		datastoreIndexToName map[int]string
		expectedCount        int
		expectedDiskIds      []string
		description          string
	}{
		{
			name: "exact name matches - datastores with original names preserved",
			datastores: []api.Datastore{
				{DiskId: "storage1"},
				{DiskId: "storage2"},
				{DiskId: "storage3"},
			},
			vms: []vsphere.VM{
				{
					Disks: []vsphere.Disk{
						{Datastore: vsphere.Ref{ID: "datastore-101"}},
						{Datastore: vsphere.Ref{ID: "datastore-102"}},
					},
				},
			},
			datastoreMapping: map[string]string{
				"storage1": "datastore-101",
				"storage2": "datastore-102",
				"storage3": "datastore-103",
			},
			datastoreIndexToName: map[int]string{
				0: "storage1",
				1: "storage2",
				2: "storage3",
			},
			expectedCount:   2,
			expectedDiskIds: []string{"storage1", "storage2"},
			description:     "Should match exactly 2 datastores used by VMs",
		},
		{
			name: "NAA identifier replacement - datastores with DiskId replaced by NAA",
			datastores: []api.Datastore{
				{DiskId: "naa.600a098038314648593f517773636465"}, // NAA replaced
				{DiskId: "naa.624a9370a7b9f7ecc01e40f70001181f"}, // NAA replaced
				{DiskId: "naa.600a0980383139544924583130316a78"}, // NAA replaced
			},
			vms: []vsphere.VM{
				{
					Disks: []vsphere.Disk{
						{Datastore: vsphere.Ref{ID: "datastore-201"}},
						{Datastore: vsphere.Ref{ID: "datastore-202"}},
					},
				},
			},
			datastoreMapping: map[string]string{
				"eco-iscsi-ds1": "datastore-201",
				"eco-iscsi-ds2": "datastore-202",
				"eco-iscsi-ds3": "datastore-203",
			},
			datastoreIndexToName: map[int]string{
				0: "eco-iscsi-ds1", // Original name before NAA replacement
				1: "eco-iscsi-ds2",
				2: "eco-iscsi-ds3",
			},
			expectedCount: 2,
			expectedDiskIds: []string{
				"naa.600a098038314648593f517773636465",
				"naa.624a9370a7b9f7ecc01e40f70001181f",
			},
			description: "Should match datastores by original name even after NAA replacement",
		},
		{
			name: "mixed storage types - NFS, SAS, iSCSI",
			datastores: []api.Datastore{
				{DiskId: "N/A", Type: "NFS"},                     // NFS datastore
				{DiskId: "mpx.vmhba0:C0:T1:L0", Type: "VMFS"},    // Local SAS
				{DiskId: "naa.60002ac0000000000000182d00021f6b"}, // iSCSI
			},
			vms: []vsphere.VM{
				{
					Disks: []vsphere.Disk{
						{Datastore: vsphere.Ref{ID: "datastore-301"}}, // NFS
						{Datastore: vsphere.Ref{ID: "datastore-302"}}, // Local SAS
					},
				},
			},
			datastoreMapping: map[string]string{
				"nfs-storage":   "datastore-301",
				"local-storage": "datastore-302",
				"iscsi-storage": "datastore-303",
			},
			datastoreIndexToName: map[int]string{
				0: "nfs-storage",
				1: "local-storage",
				2: "iscsi-storage",
			},
			expectedCount:   2,
			expectedDiskIds: []string{"N/A", "mpx.vmhba0:C0:T1:L0"},
			description:     "Should handle different storage protocol types correctly",
		},
		{
			name: "no matching datastores - VMs use different datastores",
			datastores: []api.Datastore{
				{DiskId: "storage-a"},
				{DiskId: "storage-b"},
			},
			vms: []vsphere.VM{
				{
					Disks: []vsphere.Disk{
						{Datastore: vsphere.Ref{ID: "datastore-999"}}, // Not in mapping
					},
				},
			},
			datastoreMapping: map[string]string{
				"storage-a": "datastore-401",
				"storage-b": "datastore-402",
			},
			datastoreIndexToName: map[int]string{
				0: "storage-a",
				1: "storage-b",
			},
			expectedCount:   0,
			expectedDiskIds: []string{},
			description:     "Should return empty list when no datastores match",
		},
		{
			name: "substring names should not cause false positives",
			datastores: []api.Datastore{
				{DiskId: "prod"},
				{DiskId: "production"},
				{DiskId: "prod-backup"},
			},
			vms: []vsphere.VM{
				{
					Disks: []vsphere.Disk{
						{Datastore: vsphere.Ref{ID: "datastore-501"}}, // Only "prod"
					},
				},
			},
			datastoreMapping: map[string]string{
				"prod":        "datastore-501",
				"production":  "datastore-502",
				"prod-backup": "datastore-503",
			},
			datastoreIndexToName: map[int]string{
				0: "prod",
				1: "production",
				2: "prod-backup",
			},
			expectedCount:   1,
			expectedDiskIds: []string{"prod"},
			description:     "Should match only exact names, not substrings",
		},
		{
			name: "empty inputs",
			datastores: []api.Datastore{
				{DiskId: "storage1"},
			},
			vms:                  []vsphere.VM{},
			datastoreMapping:     map[string]string{"storage1": "datastore-601"},
			datastoreIndexToName: map[int]string{0: "storage1"},
			expectedCount:        0,
			expectedDiskIds:      []string{},
			description:          "Should return empty when no VMs provided",
		},
		{
			name:       "no datastores",
			datastores: []api.Datastore{},
			vms: []vsphere.VM{
				{
					Disks: []vsphere.Disk{
						{Datastore: vsphere.Ref{ID: "datastore-701"}},
					},
				},
			},
			datastoreMapping:     map[string]string{},
			datastoreIndexToName: map[int]string{},
			expectedCount:        0,
			expectedDiskIds:      []string{},
			description:          "Should return empty when no datastores provided",
		},
		{
			name: "VMs with disks but no datastore IDs",
			datastores: []api.Datastore{
				{DiskId: "storage1"},
			},
			vms: []vsphere.VM{
				{
					Disks: []vsphere.Disk{
						{Datastore: vsphere.Ref{ID: ""}}, // Empty ID
					},
				},
			},
			datastoreMapping:     map[string]string{"storage1": "datastore-801"},
			datastoreIndexToName: map[int]string{0: "storage1"},
			expectedCount:        0,
			expectedDiskIds:      []string{},
			description:          "Should handle VMs with empty datastore IDs",
		},
		{
			name: "fallback to DiskId when no original name mapping",
			datastores: []api.Datastore{
				{DiskId: "storage-direct"},
			},
			vms: []vsphere.VM{
				{
					Disks: []vsphere.Disk{
						{Datastore: vsphere.Ref{ID: "datastore-901"}},
					},
				},
			},
			datastoreMapping:     map[string]string{"storage-direct": "datastore-901"},
			datastoreIndexToName: map[int]string{}, // No mapping available
			expectedCount:        1,
			expectedDiskIds:      []string{"storage-direct"},
			description:          "Should fallback to exact DiskId match when no index mapping exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterDatastoresByVMs(
				tt.datastores,
				tt.vms,
				tt.datastoreMapping,
				tt.datastoreIndexToName,
			)

			assert.Equal(t, tt.expectedCount, len(result), tt.description)

			if tt.expectedCount > 0 {
				resultDiskIds := make([]string, len(result))
				for i, ds := range result {
					resultDiskIds[i] = ds.DiskId
				}
				assert.ElementsMatch(t, tt.expectedDiskIds, resultDiskIds,
					"Expected DiskIds to match (order-independent)")
			}
		})
	}
}

func TestFilterDatastoresByVMs_MultipleVMs(t *testing.T) {
	datastores := []api.Datastore{
		{DiskId: "ds1"},
		{DiskId: "ds2"},
		{DiskId: "ds3"},
		{DiskId: "ds4"},
	}

	vms := []vsphere.VM{
		{
			Disks: []vsphere.Disk{
				{Datastore: vsphere.Ref{ID: "datastore-1"}},
				{Datastore: vsphere.Ref{ID: "datastore-2"}},
			},
		},
		{
			Disks: []vsphere.Disk{
				{Datastore: vsphere.Ref{ID: "datastore-2"}}, // Same as VM1
				{Datastore: vsphere.Ref{ID: "datastore-3"}},
			},
		},
		{
			Disks: []vsphere.Disk{
				{Datastore: vsphere.Ref{ID: "datastore-1"}}, // Same as VM1
			},
		},
	}

	datastoreMapping := map[string]string{
		"ds1": "datastore-1",
		"ds2": "datastore-2",
		"ds3": "datastore-3",
		"ds4": "datastore-4",
	}

	datastoreIndexToName := map[int]string{
		0: "ds1",
		1: "ds2",
		2: "ds3",
		3: "ds4",
	}

	result := FilterDatastoresByVMs(datastores, vms, datastoreMapping, datastoreIndexToName)

	assert.Equal(t, 3, len(result), "Should deduplicate and return 3 unique datastores")

	resultDiskIds := make([]string, len(result))
	for i, ds := range result {
		resultDiskIds[i] = ds.DiskId
	}
	assert.ElementsMatch(t, []string{"ds1", "ds2", "ds3"}, resultDiskIds)
}

func TestFilterNetworksByVMs(t *testing.T) {
	tests := []struct {
		name             string
		networks         []api.Network
		vms              []vsphere.VM
		networkMapping   map[string]string
		expectedCount    int
		expectedNetworks []string
		description      string
	}{
		{
			name: "NIC-based network IDs",
			networks: []api.Network{
				{Name: "network1"},
				{Name: "network2"},
				{Name: "network3"},
			},
			vms: []vsphere.VM{
				{
					NICs: []vsphere.NIC{
						{Network: vsphere.Ref{ID: "network-101"}},
						{Network: vsphere.Ref{ID: "network-102"}},
					},
				},
			},
			networkMapping: map[string]string{
				"network-101": "network1",
				"network-102": "network2",
				"network-103": "network3",
			},
			expectedCount:    2,
			expectedNetworks: []string{"network1", "network2"},
			description:      "Should match networks used by VM NICs",
		},
		{
			name: "vm.Networks-based IDs",
			networks: []api.Network{
				{Name: "vlan10"},
				{Name: "vlan20"},
				{Name: "vlan30"},
			},
			vms: []vsphere.VM{
				{
					Networks: []vsphere.Ref{
						{ID: "dvportgroup-201"},
						{ID: "dvportgroup-202"},
					},
				},
			},
			networkMapping: map[string]string{
				"dvportgroup-201": "vlan10",
				"dvportgroup-202": "vlan20",
				"dvportgroup-203": "vlan30",
			},
			expectedCount:    2,
			expectedNetworks: []string{"vlan10", "vlan20"},
			description:      "Should match networks from vm.Networks field",
		},
		{
			name: "both NICs and vm.Networks",
			networks: []api.Network{
				{Name: "mgmt"},
				{Name: "storage"},
				{Name: "vmotion"},
			},
			vms: []vsphere.VM{
				{
					NICs: []vsphere.NIC{
						{Network: vsphere.Ref{ID: "network-301"}},
					},
					Networks: []vsphere.Ref{
						{ID: "dvportgroup-302"},
					},
				},
			},
			networkMapping: map[string]string{
				"network-301":     "mgmt",
				"dvportgroup-302": "storage",
				"network-303":     "vmotion",
			},
			expectedCount:    2,
			expectedNetworks: []string{"mgmt", "storage"},
			description:      "Should collect networks from both NICs and vm.Networks",
		},
		{
			name: "networkMapping missing some IDs - graceful handling",
			networks: []api.Network{
				{Name: "known-network"},
				{Name: "another-network"},
			},
			vms: []vsphere.VM{
				{
					NICs: []vsphere.NIC{
						{Network: vsphere.Ref{ID: "network-401"}}, // In mapping
						{Network: vsphere.Ref{ID: "network-999"}}, // NOT in mapping
					},
				},
			},
			networkMapping: map[string]string{
				"network-401": "known-network",
				"network-402": "another-network",
				// network-999 is missing from mapping
			},
			expectedCount:    1,
			expectedNetworks: []string{"known-network"},
			description:      "Should gracefully handle missing networkMapping entries",
		},
		{
			name: "networks unused by VMs are excluded",
			networks: []api.Network{
				{Name: "prod-network"},
				{Name: "dev-network"},
				{Name: "test-network"},
				{Name: "unused-network"},
			},
			vms: []vsphere.VM{
				{
					NICs: []vsphere.NIC{
						{Network: vsphere.Ref{ID: "network-501"}},
						{Network: vsphere.Ref{ID: "network-502"}},
					},
				},
			},
			networkMapping: map[string]string{
				"network-501": "prod-network",
				"network-502": "dev-network",
				"network-503": "test-network",
				"network-504": "unused-network",
			},
			expectedCount:    2,
			expectedNetworks: []string{"prod-network", "dev-network"},
			description:      "Should exclude networks not used by any VM",
		},
		{
			name: "empty network IDs are ignored",
			networks: []api.Network{
				{Name: "valid-network"},
			},
			vms: []vsphere.VM{
				{
					NICs: []vsphere.NIC{
						{Network: vsphere.Ref{ID: ""}},            // Empty ID
						{Network: vsphere.Ref{ID: "network-601"}}, // Valid
					},
					Networks: []vsphere.Ref{
						{ID: ""}, // Empty ID
					},
				},
			},
			networkMapping: map[string]string{
				"network-601": "valid-network",
			},
			expectedCount:    1,
			expectedNetworks: []string{"valid-network"},
			description:      "Should ignore empty network IDs",
		},
		{
			name: "no matching networks",
			networks: []api.Network{
				{Name: "network-a"},
				{Name: "network-b"},
			},
			vms: []vsphere.VM{
				{
					NICs: []vsphere.NIC{
						{Network: vsphere.Ref{ID: "network-999"}}, // Not in mapping
					},
				},
			},
			networkMapping: map[string]string{
				"network-701": "network-a",
				"network-702": "network-b",
			},
			expectedCount:    0,
			expectedNetworks: []string{},
			description:      "Should return empty when no networks match",
		},
		{
			name:             "empty inputs",
			networks:         []api.Network{{Name: "network1"}},
			vms:              []vsphere.VM{},
			networkMapping:   map[string]string{"network-801": "network1"},
			expectedCount:    0,
			expectedNetworks: []string{},
			description:      "Should return empty when no VMs provided",
		},
		{
			name: "VMs with no NICs or Networks",
			networks: []api.Network{
				{Name: "network1"},
			},
			vms: []vsphere.VM{
				{
					NICs:     []vsphere.NIC{},
					Networks: []vsphere.Ref{},
				},
			},
			networkMapping:   map[string]string{"network-901": "network1"},
			expectedCount:    0,
			expectedNetworks: []string{},
			description:      "Should handle VMs with no network connections",
		},
		{
			name: "duplicate network IDs across NICs and vm.Networks",
			networks: []api.Network{
				{Name: "shared-network"},
				{Name: "other-network"},
			},
			vms: []vsphere.VM{
				{
					NICs: []vsphere.NIC{
						{Network: vsphere.Ref{ID: "network-1001"}},
						{Network: vsphere.Ref{ID: "network-1001"}}, // Duplicate
					},
					Networks: []vsphere.Ref{
						{ID: "network-1001"}, // Same as NICs
					},
				},
			},
			networkMapping: map[string]string{
				"network-1001": "shared-network",
				"network-1002": "other-network",
			},
			expectedCount:    1,
			expectedNetworks: []string{"shared-network"},
			description:      "Should deduplicate network IDs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterNetworksByVMs(
				tt.networks,
				tt.vms,
				tt.networkMapping,
			)

			assert.Equal(t, tt.expectedCount, len(result), tt.description)

			if tt.expectedCount > 0 {
				resultNames := make([]string, len(result))
				for i, net := range result {
					resultNames[i] = net.Name
				}
				assert.ElementsMatch(t, tt.expectedNetworks, resultNames,
					"Expected network names to match (order-independent)")
			}
		})
	}
}

func TestFilterNetworksByVMs_MultipleVMs(t *testing.T) {
	networks := []api.Network{
		{Name: "net1"},
		{Name: "net2"},
		{Name: "net3"},
		{Name: "net4"},
	}

	vms := []vsphere.VM{
		{
			NICs: []vsphere.NIC{
				{Network: vsphere.Ref{ID: "network-1"}},
			},
			Networks: []vsphere.Ref{
				{ID: "network-2"},
			},
		},
		{
			NICs: []vsphere.NIC{
				{Network: vsphere.Ref{ID: "network-2"}}, // Same as VM1
				{Network: vsphere.Ref{ID: "network-3"}},
			},
		},
		{
			NICs: []vsphere.NIC{
				{Network: vsphere.Ref{ID: "network-1"}}, // Same as VM1
			},
		},
	}

	networkMapping := map[string]string{
		"network-1": "net1",
		"network-2": "net2",
		"network-3": "net3",
		"network-4": "net4",
	}

	result := FilterNetworksByVMs(networks, vms, networkMapping)

	assert.Equal(t, 3, len(result), "Should deduplicate and return 3 unique networks")

	resultNames := make([]string, len(result))
	for i, net := range result {
		resultNames[i] = net.Name
	}
	assert.ElementsMatch(t, []string{"net1", "net2", "net3"}, resultNames)
}

func TestFilterInfraDataByClusterID_NilHostsGuard(t *testing.T) {
	// Test that FilterInfraDataByClusterID handles nil Hosts pointer gracefully
	infraData := InfrastructureData{
		Datastores:      []api.Datastore{{DiskId: "ds1"}},
		Networks:        []api.Network{{Name: "net1"}},
		HostPowerStates: map[string]int{"green": 5},
		Hosts:           nil, // Nil pointer
	}

	clusterMapping := map[string]string{
		"host-1": "cluster-1",
	}

	vms := []vsphere.VM{
		{
			Disks: []vsphere.Disk{
				{Datastore: vsphere.Ref{ID: "datastore-1"}},
			},
		},
	}

	datastoreMapping := map[string]string{
		"ds1": "datastore-1",
	}

	datastoreIndexToName := map[int]string{
		0: "ds1",
	}

	networkMapping := map[string]string{
		"network-1": "net1",
	}

	hostIDToPowerState := map[string]string{}

	// Should not panic
	result := FilterInfraDataByClusterID(
		infraData,
		"cluster-1",
		clusterMapping,
		vms,
		datastoreMapping,
		datastoreIndexToName,
		networkMapping,
		hostIDToPowerState,
	)

	assert.NotNil(t, result.Hosts, "Should return non-nil Hosts pointer")
	assert.Equal(t, 0, len(*result.Hosts), "Should return empty Hosts slice when input is nil")
	assert.Equal(t, 0, result.TotalHosts, "TotalHosts should be 0")
	assert.Equal(t, map[string]int{"green": 0}, result.HostPowerStates, "Should return zero green hosts")
}

func TestCalculateHostPowerStatesForCluster(t *testing.T) {
	// Test the real power state calculation from vHost data
	tests := []struct {
		name               string
		hosts              []api.Host
		hostIDToPowerState map[string]string
		expectedMap        map[string]int
		description        string
	}{
		{
			name: "actual power states from vHost data",
			hosts: []api.Host{
				{Id: stringPtr("host-1"), CpuCores: intPtr(32)},
				{Id: stringPtr("host-2"), CpuCores: intPtr(32)},
				{Id: stringPtr("host-3"), CpuCores: intPtr(32)},
				{Id: stringPtr("host-4"), CpuCores: intPtr(32)},
			},
			hostIDToPowerState: map[string]string{
				"host-1": "green",
				"host-2": "green",
				"host-3": "red",
				"host-4": "yellow",
			},
			expectedMap: map[string]int{"green": 2, "red": 1, "yellow": 1},
			description: "Should reflect actual power states from vHost sheet",
		},
		{
			name: "host missing from power state map defaults to green",
			hosts: []api.Host{
				{Id: stringPtr("host-1"), CpuCores: intPtr(32)},
				{Id: stringPtr("host-2"), CpuCores: intPtr(32)},
			},
			hostIDToPowerState: map[string]string{
				"host-1": "red",
				// host-2 is missing
			},
			expectedMap: map[string]int{"red": 1, "green": 1},
			description: "Should default to green if host not in power state map",
		},
		{
			name: "host with nil ID defaults to green",
			hosts: []api.Host{
				{Id: nil, CpuCores: intPtr(32)},
				{Id: stringPtr("host-1"), CpuCores: intPtr(32)},
			},
			hostIDToPowerState: map[string]string{
				"host-1": "yellow",
			},
			expectedMap: map[string]int{"green": 1, "yellow": 1},
			description: "Should default to green if host ID is nil",
		},
		{
			name:               "no hosts returns zero green",
			hosts:              []api.Host{},
			hostIDToPowerState: map[string]string{},
			expectedMap:        map[string]int{"green": 0},
			description:        "Empty hosts should return zero green count",
		},
		{
			name:               "nil hosts slice returns zero green",
			hosts:              nil,
			hostIDToPowerState: map[string]string{},
			expectedMap:        map[string]int{"green": 0},
			description:        "Nil hosts should return zero green count",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateHostPowerStatesForCluster(tt.hosts, tt.hostIDToPowerState)
			assert.Equal(t, tt.expectedMap, result, tt.description)
		})
	}
}

func TestCalculateCpuOverCommitmentForCluster(t *testing.T) {
	tests := []struct {
		name                   string
		hosts                  []api.Host
		vms                    []vsphere.VM
		expectedAllocatedVCpus int
		expectedPhysicalCores  int
		description            string
	}{
		{
			name: "standard cluster with powered-on VMs",
			hosts: []api.Host{
				{Id: stringPtr("host-1"), CpuCores: intPtr(32)},
				{Id: stringPtr("host-2"), CpuCores: intPtr(32)},
			},
			vms: []vsphere.VM{
				{PowerState: "poweredOn", CpuCount: 4},
				{PowerState: "poweredOn", CpuCount: 8},
				{PowerState: "poweredOff", CpuCount: 2}, // should be excluded
			},
			expectedAllocatedVCpus: 12, // 4 + 8
			expectedPhysicalCores:  64, // 32 + 32
			description:            "Should sum vCPUs from powered-on VMs only",
		},
		{
			name: "cluster with all VMs powered off",
			hosts: []api.Host{
				{Id: stringPtr("host-1"), CpuCores: intPtr(16)},
			},
			vms: []vsphere.VM{
				{PowerState: "poweredOff", CpuCount: 4},
				{PowerState: "suspended", CpuCount: 8},
			},
			expectedAllocatedVCpus: 0,
			expectedPhysicalCores:  16,
			description:            "Should have zero allocated vCPUs when all VMs are off",
		},
		{
			name: "cluster with hosts missing CPU cores data",
			hosts: []api.Host{
				{Id: stringPtr("host-1"), CpuCores: intPtr(32)},
				{Id: stringPtr("host-2"), CpuCores: nil}, // missing CPU data
				{Id: stringPtr("host-3"), CpuCores: intPtr(16)},
			},
			vms: []vsphere.VM{
				{PowerState: "poweredOn", CpuCount: 4},
			},
			expectedAllocatedVCpus: 4,
			expectedPhysicalCores:  48, // 32 + 0 + 16
			description:            "Should handle hosts with nil CpuCores gracefully",
		},
		{
			name:                   "empty cluster - no hosts or VMs",
			hosts:                  []api.Host{},
			vms:                    []vsphere.VM{},
			expectedAllocatedVCpus: 0,
			expectedPhysicalCores:  0,
			description:            "Should return zeros for empty cluster",
		},
		{
			name: "hosts only - no VMs",
			hosts: []api.Host{
				{Id: stringPtr("host-1"), CpuCores: intPtr(64)},
			},
			vms:                    []vsphere.VM{},
			expectedAllocatedVCpus: 0,
			expectedPhysicalCores:  64,
			description:            "Should return zero vCPUs when no VMs exist",
		},
		{
			name:  "VMs only - no hosts",
			hosts: []api.Host{},
			vms: []vsphere.VM{
				{PowerState: "poweredOn", CpuCount: 8},
			},
			expectedAllocatedVCpus: 8,
			expectedPhysicalCores:  0,
			description:            "Should return zero cores when no hosts exist",
		},
		{
			name: "high overcommitment ratio",
			hosts: []api.Host{
				{Id: stringPtr("host-1"), CpuCores: intPtr(8)},
			},
			vms: []vsphere.VM{
				{PowerState: "poweredOn", CpuCount: 4},
				{PowerState: "poweredOn", CpuCount: 4},
				{PowerState: "poweredOn", CpuCount: 8},
				{PowerState: "poweredOn", CpuCount: 8},
			},
			expectedAllocatedVCpus: 24, // 4 + 4 + 8 + 8 = 24 (3:1 ratio)
			expectedPhysicalCores:  8,
			description:            "Should correctly calculate high overcommitment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateCpuOverCommitmentForCluster(tt.hosts, tt.vms)
			assert.Equal(t, tt.expectedAllocatedVCpus, result.AllocatedVCpus, tt.description+" - AllocatedVCpus")
			assert.Equal(t, tt.expectedPhysicalCores, result.PhysicalCores, tt.description+" - PhysicalCores")
		})
	}
}

// Helper functions for tests
func intPtr(i int) *int {
	return &i
}

func stringPtr(s string) *string {
	return &s
}
