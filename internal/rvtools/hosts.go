package rvtools

import (
	"fmt"
	"strings"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
)

func processHostInfo(rows [][]string, inventory *api.Inventory) error {
	if len(rows) <= 1 {
		return fmt.Errorf("no valid host data available")
	}
	
	colMap := make(map[string]int)
	for i, header := range rows[0] {
		key := strings.ToLower(strings.TrimSpace(header))
		colMap[key] = i
	}

	if inventory.Infra.HostPowerStates == nil {
		inventory.Infra.HostPowerStates = make(map[string]int)
	}

	clusterCol := -1
	if idx, exists := colMap["cluster"]; exists {
		clusterCol = idx
	} else {
		for header, idx := range colMap {
			if strings.Contains(header, "cluster") {
				clusterCol = idx
				break
			}
		}
	}

	hostCount := 0
	uniqueClusters := make(map[string]bool)
	clusterHosts := make(map[string]int)

	for i := 1; i < len(rows); i++ {
		row := rows[i]
		if len(row) == 0 {
			continue
		}

		hostCount++

		if idx, exists := colMap["config status"]; exists && idx < len(row) {
			stateColor := row[idx]
			switch stateColor {
			case "red", "yellow", "green", "gray":
				inventory.Infra.HostPowerStates[stateColor]++
			default:
				inventory.Infra.HostPowerStates["green"]++
			}
		} else {
			inventory.Infra.HostPowerStates["green"]++
		}

		if clusterCol >= 0 && clusterCol < len(row) {
			clusterName := strings.TrimSpace(row[clusterCol])
			if clusterName != "" {
				uniqueClusters[clusterName] = true
				clusterHosts[clusterName]++
			} else {
				clusterHosts["default"]++
			}
		} else {
			clusterHosts["default"]++
		}
	}

	inventory.Infra.TotalHosts = hostCount

	inventory.Infra.TotalClusters = len(uniqueClusters)
	if inventory.Infra.TotalClusters == 0 && hostCount > 0 {
		inventory.Infra.TotalClusters = 1
	}

	hostsPerCluster := []int{}
	for _, count := range clusterHosts {
		hostsPerCluster = append(hostsPerCluster, count)
	}
	inventory.Infra.HostsPerCluster = hostsPerCluster
	
	return nil
}
