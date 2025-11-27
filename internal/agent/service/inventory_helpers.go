package service

import (
	"sort"
)

func CalculateHostsPerCluster(clusterToHosts map[string]map[string]struct{}) []int {
	if len(clusterToHosts) == 0 {
		return []int{}
	}

	// Sort cluster names for consistent ordering
	clusterNames := make([]string, 0, len(clusterToHosts))
	for cluster := range clusterToHosts {
		clusterNames = append(clusterNames, cluster)
	}
	sort.Strings(clusterNames)

	hostsPerCluster := make([]int, 0, len(clusterNames))
	for _, cluster := range clusterNames {
		hostsPerCluster = append(hostsPerCluster, len(clusterToHosts[cluster]))
	}

	return hostsPerCluster
}

func CalculateVMsPerCluster(clusterToVMs map[string]map[string]struct{}) []int {
	if len(clusterToVMs) == 0 {
		return []int{}
	}

	// Sort cluster names for consistent ordering
	clusterNames := make([]string, 0, len(clusterToVMs))
	for cluster := range clusterToVMs {
		clusterNames = append(clusterNames, cluster)
	}
	sort.Strings(clusterNames)

	vmsPerCluster := make([]int, 0, len(clusterNames))
	for _, cluster := range clusterNames {
		vmsPerCluster = append(vmsPerCluster, len(clusterToVMs[cluster]))
	}

	return vmsPerCluster
}

func CalculateClustersPerDatacenter(datacenterToClusters map[string]map[string]struct{}) []int {
	if len(datacenterToClusters) == 0 {
		return []int{}
	}

	// Sort datacenter names for consistent ordering
	datacenterNames := make([]string, 0, len(datacenterToClusters))
	for datacenter := range datacenterToClusters {
		datacenterNames = append(datacenterNames, datacenter)
	}
	sort.Strings(datacenterNames)

	clustersPerDatacenter := make([]int, 0, len(datacenterNames))
	for _, datacenter := range datacenterNames {
		clustersPerDatacenter = append(clustersPerDatacenter, len(datacenterToClusters[datacenter]))
	}

	return clustersPerDatacenter
}
