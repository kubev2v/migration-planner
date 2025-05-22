package rvtools

import (
	"fmt"
	"strings"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/xuri/excelize/v2"
)

func processHostInfo(excelFile *excelize.File, inventory *api.Inventory, sheetName string) error {
	rows, err := excelFile.GetRows(sheetName)
	if err != nil || len(rows) <= 1 {
		return fmt.Errorf("no valid host data available")
	}
	
	colMap := make(map[string]int)
	for i, header := range rows[0] {
		headerTrimmed := strings.TrimSpace(header)
		colMap[header] = i
		colMap[headerTrimmed] = i
		colMap[strings.ToLower(header)] = i
		colMap[strings.ToLower(headerTrimmed)] = i
	}

	clusterCol := -1
	if idx, exists := colMap["Cluster"]; exists {
		clusterCol = idx
	} else {
		for header, idx := range colMap {
			if strings.Contains(strings.ToLower(header), "cluster") {
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

		if idx, exists := colMap["Config Status"]; exists && idx < len(row) {
			stateColor := strings.ToLower(strings.TrimSpace(row[idx]))
			if stateColor == "red" || stateColor == "yellow" || stateColor == "green" || stateColor == "gray" {
				inventory.Infra.HostPowerStates[stateColor]++
			} else {
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
