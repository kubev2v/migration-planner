package service

import (
	vsphere "github.com/kubev2v/forklift/pkg/controller/provider/model/vsphere"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
)

type ClusterIDMapping struct {
	VMToClusterID   map[string]string // VM Name -> Cluster ID
	HostToClusterID map[string]string // Host Object ID -> Cluster ID
	ClusterIDs      []string          // List of unique cluster IDs (sorted)
}

type ClusteredInventoryResponse struct {
	VCenterID string                        `json:"vcenter_id"`
	Clusters  map[string]*api.InventoryData `json:"clusters"` // Keys are cluster IDs
	VCenter   *api.InventoryData            `json:"vcenter"`  // Aggregated vcenter inventory
}

func FilterVMsByClusterID(vms []vsphere.VM, clusterID string, vmToClusterID map[string]string) []vsphere.VM {
	// Count first to allocate exact capacity
	count := 0
	for _, vm := range vms {
		if vmClusterID, exists := vmToClusterID[vm.Name]; exists && vmClusterID == clusterID {
			count++
		}
	}

	// Allocate with exact capacity
	filtered := make([]vsphere.VM, 0, count)
	for _, vm := range vms {
		if vmClusterID, exists := vmToClusterID[vm.Name]; exists && vmClusterID == clusterID {
			filtered = append(filtered, vm)
		}
	}

	return filtered
}

func FilterHostsByClusterID(hosts []api.Host, clusterID string, hostToClusterID map[string]string) []api.Host {
	// Count first to allocate exact capacity
	count := 0
	for _, host := range hosts {
		if host.Id != nil {
			if hostClusterID, exists := hostToClusterID[*host.Id]; exists && hostClusterID == clusterID {
				count++
			}
		}
	}

	// Allocate with exact capacity
	filtered := make([]api.Host, 0, count)
	for _, host := range hosts {
		if host.Id != nil {
			if hostClusterID, exists := hostToClusterID[*host.Id]; exists && hostClusterID == clusterID {
				filtered = append(filtered, host)
			}
		}
	}

	return filtered
}

func FilterInfraDataByClusterID(
	infraData InfrastructureData,
	clusterID string,
	hostToClusterID map[string]string,
	clusterVMs []vsphere.VM,
	datastoreMapping map[string]string,
	datastoreIndexToName map[int]string,
	networkMapping map[string]string,
	hostIDToPowerState map[string]string,
) InfrastructureData {
	// Guard against nil Hosts pointer to make this helper more robust for reuse
	var baseHosts []api.Host
	if infraData.Hosts != nil {
		baseHosts = *infraData.Hosts
	}
	clusterHosts := FilterHostsByClusterID(baseHosts, clusterID, hostToClusterID)

	clusterHostPowerStates := CalculateHostPowerStatesForCluster(clusterHosts, hostIDToPowerState)

	clusterDatastores := FilterDatastoresByVMs(infraData.Datastores, clusterVMs, datastoreMapping, datastoreIndexToName)

	clusterNetworks := FilterNetworksByVMs(infraData.Networks, clusterVMs, networkMapping)

	return InfrastructureData{
		Datastores:            clusterDatastores,
		Networks:              clusterNetworks,
		HostPowerStates:       clusterHostPowerStates,
		Hosts:                 &clusterHosts,
		HostsPerCluster:       []int{len(clusterHosts)},
		ClustersPerDatacenter: []int{1}, // Single cluster
		TotalHosts:            len(clusterHosts),
		TotalClusters:         1,
		TotalDatacenters:      1,
		VmsPerCluster:         []int{len(clusterVMs)},
	}
}

func CalculateHostPowerStatesForCluster(clusterHosts []api.Host, hostIDToPowerState map[string]string) map[string]int {
	powerStates := make(map[string]int)

	for _, host := range clusterHosts {
		if host.Id == nil {
			// If host ID is missing, default to green
			powerStates["green"]++
			continue
		}

		hostID := *host.Id
		if powerState, exists := hostIDToPowerState[hostID]; exists {
			powerStates[powerState]++
		} else {
			// If power state is not found in the map, default to green
			powerStates["green"]++
		}
	}

	// Ensure we return at least an empty green count if no hosts
	if len(powerStates) == 0 {
		powerStates["green"] = 0
	}

	return powerStates
}

func FilterDatastoresByVMs(
	datastores []api.Datastore,
	vms []vsphere.VM,
	datastoreMapping map[string]string,
	datastoreIndexToName map[int]string,
) []api.Datastore {
	// Collect datastore object IDs used by VMs
	usedObjectIDs := make(map[string]struct{})
	for _, vm := range vms {
		for _, disk := range vm.Disks {
			if disk.Datastore.ID != "" {
				usedObjectIDs[disk.Datastore.ID] = struct{}{}
			}
		}
	}

	usedDatastoreNames := make(map[string]struct{})
	for name, objID := range datastoreMapping {
		if _, isUsed := usedObjectIDs[objID]; isUsed {
			usedDatastoreNames[name] = struct{}{}
		}
	}

	// Filter datastores by matching original names
	filtered := make([]api.Datastore, 0, len(usedDatastoreNames))
	for idx, ds := range datastores {
		// Get the original name before NAA/path replacement
		originalName, hasOriginalName := datastoreIndexToName[idx]

		if hasOriginalName {
			// Match against original name
			if _, used := usedDatastoreNames[originalName]; used {
				filtered = append(filtered, ds)
			}
		} else {
			// Fallback: if we don't have the original name mapping, try exact match with DiskId
			if _, used := usedDatastoreNames[ds.DiskId]; used {
				filtered = append(filtered, ds)
			}
		}
	}

	return filtered
}

func FilterNetworksByVMs(networks []api.Network, vms []vsphere.VM, networkMapping map[string]string) []api.Network {
	// Collect network object IDs used by VMs
	networkObjectIDSet := make(map[string]struct{})
	for _, vm := range vms {
		for _, nic := range vm.NICs {
			if nic.Network.ID != "" {
				networkObjectIDSet[nic.Network.ID] = struct{}{}
			}
		}
		for _, net := range vm.Networks {
			if net.ID != "" {
				networkObjectIDSet[net.ID] = struct{}{}
			}
		}
	}

	// Build set of network names used by this cluster
	usedNetworkNames := make(map[string]struct{})
	for objectID := range networkObjectIDSet {
		if netName, exists := networkMapping[objectID]; exists {
			usedNetworkNames[netName] = struct{}{}
		}
	}

	// Filter networks by name
	filtered := make([]api.Network, 0, len(usedNetworkNames))
	for _, network := range networks {
		if _, used := usedNetworkNames[network.Name]; used {
			filtered = append(filtered, network)
		}
	}

	return filtered
}
