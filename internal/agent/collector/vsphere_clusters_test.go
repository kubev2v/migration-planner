package collector

import (
	"testing"

	vspheremodel "github.com/kubev2v/forklift/pkg/controller/provider/model/vsphere"
	"github.com/stretchr/testify/assert"
)

func TestExtractVSphereClusterIDMapping(t *testing.T) {
	tests := []struct {
		name     string
		vms      []vspheremodel.VM
		hosts    []vspheremodel.Host
		clusters []vspheremodel.Cluster
		want     struct {
			hostToClusterCount int
			clusterIDsCount    int
			clusterIDs         []string
			vmsByClusterCount  int // Number of clusters that have VMs
		}
	}{
		{
			name: "Single cluster with VMs and hosts",
			vms: []vspheremodel.VM{
				{UUID: "vm-1", Host: "host-1"},
				{UUID: "vm-2", Host: "host-1"},
				{UUID: "vm-3", Host: "host-2"},
			},
			hosts: []vspheremodel.Host{
				{Base: vspheremodel.Base{ID: "host-1"}, Cluster: "domain-c8"},
				{Base: vspheremodel.Base{ID: "host-2"}, Cluster: "domain-c8"},
			},
			clusters: []vspheremodel.Cluster{
				{Base: vspheremodel.Base{ID: "domain-c8"}},
			},
			want: struct {
				hostToClusterCount int
				clusterIDsCount    int
				clusterIDs         []string
				vmsByClusterCount  int
			}{
				hostToClusterCount: 2,
				clusterIDsCount:    1,
				clusterIDs:         []string{"domain-c8"},
				vmsByClusterCount:  1, // 1 cluster has VMs
			},
		},
		{
			name: "Multiple clusters",
			vms: []vspheremodel.VM{
				{UUID: "vm-1", Host: "host-1"},
				{UUID: "vm-2", Host: "host-2"},
				{UUID: "vm-3", Host: "host-3"},
			},
			hosts: []vspheremodel.Host{
				{Base: vspheremodel.Base{ID: "host-1"}, Cluster: "domain-c8"},
				{Base: vspheremodel.Base{ID: "host-2"}, Cluster: "domain-c9"},
				{Base: vspheremodel.Base{ID: "host-3"}, Cluster: "domain-c9"},
			},
			clusters: []vspheremodel.Cluster{
				{Base: vspheremodel.Base{ID: "domain-c8"}},
				{Base: vspheremodel.Base{ID: "domain-c9"}},
			},
			want: struct {
				hostToClusterCount int
				clusterIDsCount    int
				clusterIDs         []string
				vmsByClusterCount  int
			}{
				hostToClusterCount: 3,
				clusterIDsCount:    2,
				clusterIDs:         []string{"domain-c8", "domain-c9"},
				vmsByClusterCount:  2, // 2 clusters have VMs
			},
		},
		{
			name: "Cluster with hosts but no VMs",
			vms:  []vspheremodel.VM{},
			hosts: []vspheremodel.Host{
				{Base: vspheremodel.Base{ID: "host-1"}, Cluster: "domain-c8"},
				{Base: vspheremodel.Base{ID: "host-2"}, Cluster: "domain-c8"},
			},
			clusters: []vspheremodel.Cluster{
				{Base: vspheremodel.Base{ID: "domain-c8"}},
			},
			want: struct {
				hostToClusterCount int
				clusterIDsCount    int
				clusterIDs         []string
				vmsByClusterCount  int
			}{
				hostToClusterCount: 2,
				clusterIDsCount:    1,
				clusterIDs:         []string{"domain-c8"},
				vmsByClusterCount:  0, // No VMs in any cluster
			},
		},
		{
			name: "VMs on unmapped hosts are ignored",
			vms: []vspheremodel.VM{
				{UUID: "vm-1", Host: "host-1"},
				{UUID: "vm-2", Host: "host-unknown"},
			},
			hosts: []vspheremodel.Host{
				{Base: vspheremodel.Base{ID: "host-1"}, Cluster: "domain-c8"},
			},
			clusters: []vspheremodel.Cluster{
				{Base: vspheremodel.Base{ID: "domain-c8"}},
			},
			want: struct {
				hostToClusterCount int
				clusterIDsCount    int
				clusterIDs         []string
				vmsByClusterCount  int
			}{
				hostToClusterCount: 1,
				clusterIDsCount:    1,
				clusterIDs:         []string{"domain-c8"},
				vmsByClusterCount:  1, // Only 1 cluster has VMs (vm-2 is unmapped)
			},
		},
		{
			name: "Empty UUIDs and IDs are ignored",
			vms: []vspheremodel.VM{
				{UUID: "", Host: "host-1"},
				{UUID: "vm-1", Host: ""},
				{UUID: "vm-2", Host: "host-1"},
			},
			hosts: []vspheremodel.Host{
				{Base: vspheremodel.Base{ID: ""}, Cluster: "domain-c8"},
				{Base: vspheremodel.Base{ID: "host-1"}, Cluster: ""},
				{Base: vspheremodel.Base{ID: "host-2"}, Cluster: "domain-c8"},
			},
			clusters: []vspheremodel.Cluster{
				{Base: vspheremodel.Base{ID: "domain-c8"}},
			},
			want: struct {
				hostToClusterCount int
				clusterIDsCount    int
				clusterIDs         []string
				vmsByClusterCount  int
			}{
				hostToClusterCount: 1, // Only host-2 has both ID and cluster
				clusterIDsCount:    1,
				clusterIDs:         []string{"domain-c8"},
				vmsByClusterCount:  0, // No valid VM mappings
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, vmsByCluster := ExtractVSphereClusterIDMapping(tt.vms, tt.hosts, tt.clusters)

			assert.Nil(t, got.VMToClusterID, "VMToClusterID should be nil for vSphere (not used)")
			assert.Equal(t, tt.want.hostToClusterCount, len(got.HostToClusterID), "HostToClusterID count mismatch")
			assert.Equal(t, tt.want.clusterIDsCount, len(got.ClusterIDs), "ClusterIDs count mismatch")
			assert.Equal(t, tt.want.clusterIDs, got.ClusterIDs, "ClusterIDs mismatch")
			assert.Equal(t, tt.want.vmsByClusterCount, len(vmsByCluster), "vmsByCluster count mismatch")
		})
	}
}

func TestBuildHostMappings(t *testing.T) {
	tests := []struct {
		name              string
		hosts             []vspheremodel.Host
		wantHostToCluster map[string]string
		wantHostToPower   map[string]string
	}{
		{
			name: "Valid hosts with cluster and power state",
			hosts: []vspheremodel.Host{
				{Base: vspheremodel.Base{ID: "host-1"}, Cluster: "domain-c8", Status: "green"},
				{Base: vspheremodel.Base{ID: "host-2"}, Cluster: "domain-c9", Status: "yellow"},
			},
			wantHostToCluster: map[string]string{
				"host-1": "domain-c8",
				"host-2": "domain-c9",
			},
			wantHostToPower: map[string]string{
				"host-1": "green",
				"host-2": "yellow",
			},
		},
		{
			name: "Empty cluster is ignored but power state is captured",
			hosts: []vspheremodel.Host{
				{Base: vspheremodel.Base{ID: ""}, Cluster: "domain-c8", Status: "green"},
				{Base: vspheremodel.Base{ID: "host-1"}, Cluster: "", Status: "green"},
				{Base: vspheremodel.Base{ID: "host-2"}, Cluster: "domain-c8", Status: "yellow"},
			},
			wantHostToCluster: map[string]string{
				"host-2": "domain-c8",
			},
			wantHostToPower: map[string]string{
				"host-1": "green",
				"host-2": "yellow",
			},
		},
		{
			name:              "Empty hosts",
			hosts:             []vspheremodel.Host{},
			wantHostToCluster: map[string]string{},
			wantHostToPower:   map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCluster, gotPower := buildHostMappings(tt.hosts)
			assert.Equal(t, tt.wantHostToCluster, gotCluster, "hostToClusterID mismatch")
			assert.Equal(t, tt.wantHostToPower, gotPower, "hostIDToPowerState mismatch")
		})
	}
}

func TestBuildVMsByClusterMap(t *testing.T) {
	hostToClusterID := map[string]string{
		"host-1": "domain-c8",
		"host-2": "domain-c9",
	}

	tests := []struct {
		name string
		vms  []vspheremodel.VM
		want map[string][]vspheremodel.VM
	}{
		{
			name: "Valid VMs grouped by cluster",
			vms: []vspheremodel.VM{
				{UUID: "vm-1", Host: "host-1"},
				{UUID: "vm-2", Host: "host-2"},
				{UUID: "vm-3", Host: "host-1"},
			},
			want: map[string][]vspheremodel.VM{
				"domain-c8": {
					{UUID: "vm-1", Host: "host-1"},
					{UUID: "vm-3", Host: "host-1"},
				},
				"domain-c9": {
					{UUID: "vm-2", Host: "host-2"},
				},
			},
		},
		{
			name: "VMs with unmapped hosts are ignored",
			vms: []vspheremodel.VM{
				{UUID: "vm-1", Host: "host-1"},
				{UUID: "vm-2", Host: "host-unknown"},
			},
			want: map[string][]vspheremodel.VM{
				"domain-c8": {
					{UUID: "vm-1", Host: "host-1"},
				},
			},
		},
		{
			name: "Empty UUID or host are ignored",
			vms: []vspheremodel.VM{
				{UUID: "", Host: "host-1"},
				{UUID: "vm-1", Host: ""},
				{UUID: "vm-2", Host: "host-1"},
			},
			want: map[string][]vspheremodel.VM{
				"domain-c8": {
					{UUID: "vm-2", Host: "host-1"},
				},
			},
		},
		{
			name: "Empty VMs",
			vms:  []vspheremodel.VM{},
			want: map[string][]vspheremodel.VM{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVMsByCluster := buildVMsByClusterMap(tt.vms, hostToClusterID)

			// Check vmsByCluster map
			assert.Equal(t, len(tt.want), len(gotVMsByCluster), "cluster count mismatch")
			for clusterID, expectedVMs := range tt.want {
				gotVMs, ok := gotVMsByCluster[clusterID]
				assert.True(t, ok, "expected cluster %s not found", clusterID)
				assert.ElementsMatch(t, expectedVMs, gotVMs, "VMs mismatch for cluster %s", clusterID)
			}
		})
	}
}

// TestExtractVSphereClusterIDMapping_ClusterIDs verifies that cluster IDs are extracted correctly
// directly from the clusters list (testing the optimization)
func TestExtractVSphereClusterIDMapping_ClusterIDs(t *testing.T) {
	tests := []struct {
		name           string
		clusters       []vspheremodel.Cluster
		wantClusterIDs []string
	}{
		{
			name: "Multiple clusters sorted",
			clusters: []vspheremodel.Cluster{
				{Base: vspheremodel.Base{ID: "domain-c9"}},
				{Base: vspheremodel.Base{ID: "domain-c8"}},
				{Base: vspheremodel.Base{ID: "domain-c10"}},
			},
			wantClusterIDs: []string{"domain-c10", "domain-c8", "domain-c9"},
		},
		{
			name: "Empty cluster IDs are ignored",
			clusters: []vspheremodel.Cluster{
				{Base: vspheremodel.Base{ID: ""}},
				{Base: vspheremodel.Base{ID: "domain-c8"}},
			},
			wantClusterIDs: []string{"domain-c8"},
		},
		{
			name:           "Empty clusters list",
			clusters:       []vspheremodel.Cluster{},
			wantClusterIDs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapping, _, _ := ExtractVSphereClusterIDMapping([]vspheremodel.VM{}, []vspheremodel.Host{}, tt.clusters)

			assert.Equal(t, len(tt.wantClusterIDs), len(mapping.ClusterIDs), "ClusterIDs length mismatch")

			// Verify sorting
			for i := 1; i < len(mapping.ClusterIDs); i++ {
				assert.True(t, mapping.ClusterIDs[i-1] < mapping.ClusterIDs[i], "ClusterIDs should be sorted")
			}
		})
	}
}
