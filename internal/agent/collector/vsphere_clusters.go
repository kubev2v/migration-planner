package collector

import (
	"sort"

	vspheremodel "github.com/kubev2v/forklift/pkg/controller/provider/model/vsphere"
	"github.com/kubev2v/migration-planner/internal/agent/service"
)

// ExtractVSphereClusterIDMapping extracts cluster IDs and creates mappings from vSphere data.
func ExtractVSphereClusterIDMapping(
	vms []vspheremodel.VM,
	hosts []vspheremodel.Host,
	clusters []vspheremodel.Cluster,
) (service.ClusterIDMapping, map[string]string, map[string][]vspheremodel.VM) {
	clusterIDs := make([]string, 0, len(clusters))
	for _, cluster := range clusters {
		if cluster.ID != "" {
			clusterIDs = append(clusterIDs, cluster.ID)
		}
	}
	sort.Strings(clusterIDs)

	hostToClusterID, hostIDToPowerState := buildHostMappings(hosts)

	vmsByCluster := buildVMsByClusterMap(vms, hostToClusterID)

	clusterMapping := service.ClusterIDMapping{
		VMToClusterID:   nil, // Not used by vSphere (only needed for RVTools)
		HostToClusterID: hostToClusterID,
		ClusterIDs:      clusterIDs,
	}

	return clusterMapping, hostIDToPowerState, vmsByCluster
}

func buildHostMappings(hosts []vspheremodel.Host) (hostToClusterID, hostIDToPowerState map[string]string) {
	hostToClusterID = make(map[string]string, len(hosts))
	hostIDToPowerState = make(map[string]string, len(hosts))

	for _, host := range hosts {
		if host.ID != "" {
			if host.Cluster != "" {
				hostToClusterID[host.ID] = host.Cluster
			}
			hostIDToPowerState[host.ID] = host.Status
		}
	}

	return hostToClusterID, hostIDToPowerState
}

func buildVMsByClusterMap(vms []vspheremodel.VM, hostToClusterID map[string]string) map[string][]vspheremodel.VM {
	vmsByCluster := make(map[string][]vspheremodel.VM)

	for _, vm := range vms {
		if vm.UUID != "" && vm.Host != "" {
			if clusterID, ok := hostToClusterID[vm.Host]; ok && clusterID != "" {
				vmsByCluster[clusterID] = append(vmsByCluster[clusterID], vm)
			}
		}
	}

	return vmsByCluster
}
