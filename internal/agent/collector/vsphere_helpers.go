package collector

import (
	vspheremodel "github.com/kubev2v/forklift/pkg/controller/provider/model/vsphere"
)

func buildDatastoreMappings(datastores []vspheremodel.Datastore) (indexToName map[int]string, nameToID map[string]string) {
	indexToName = make(map[int]string, len(datastores))
	nameToID = make(map[string]string, len(datastores))

	for i, ds := range datastores {
		indexToName[i] = ds.Name
		if ds.Name != "" && ds.ID != "" {
			nameToID[ds.Name] = ds.ID
		}
	}

	return indexToName, nameToID
}

func buildNetworkMappingFromVSphere(networks []vspheremodel.Network) map[string]string {
	mapping := make(map[string]string, len(networks))
	for _, network := range networks {
		if network.ID != "" && network.Name != "" {
			mapping[network.ID] = network.Name
		}
	}
	return mapping
}
