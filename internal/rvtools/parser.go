package rvtools

import (
	"bytes"
	"fmt"
	"slices"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/xuri/excelize/v2"
	"go.uber.org/zap"
)

func ParseRVTools(rvtoolsContent []byte) (*api.Inventory, error) {
	excelFile, err := excelize.OpenReader(bytes.NewReader(rvtoolsContent))
	if err != nil {
		return nil, fmt.Errorf("error opening Excel file: %v", err)
	}
	defer excelFile.Close()

	sheets := excelFile.GetSheetList()

	inventory := api.Inventory{}

	inventory.Infra.Datastores = []api.Datastore{}
	inventory.Infra.HostsPerCluster = []int{}
	inventory.Vms.MigrationWarnings = api.MigrationIssues{}
	inventory.Vms.NotMigratableReasons = []api.MigrationIssue{}

	if slices.Contains(sheets, "vInfo") {
		rows, err := excelFile.GetRows("vInfo")
		if err != nil {
			zap.S().Warnf("Could not read vInfo sheet: %v", err)
		} else {
			vcenterUUID, _ := extractVCenterUUID(rows)
			inventory.Vcenter.Id = vcenterUUID
			vms, err := processVMInfo(rows)
			if err != nil {
				zap.S().Warnf("VM processing failed: %v", err)
			} else if len(vms) > 0 {
				fillInventoryWithVMData(vms, &inventory)
			}
		}
	}

	if slices.Contains(sheets, "vHost") {
		rows, err := excelFile.GetRows("vHost")
		if err != nil {
			zap.S().Warnf("Could not read vHost sheet: %v", err)
			resetHostInfo(&inventory)
		} else {
			err = processHostInfo(rows, &inventory)
			if err != nil {
				zap.S().Warnf("Host processing failed: %v", err)
				resetHostInfo(&inventory)
			}
		}
	}

	if slices.Contains(sheets, "vDatastore") {
		rows, err := excelFile.GetRows("vDatastore")
		if err != nil {
			inventory.Infra.Datastores = []api.Datastore{}
		} else {
			err = processDatastoreInfo(rows, &inventory)
			if err != nil {
				inventory.Infra.Datastores = []api.Datastore{}
			}
		}
	}

	var dvswitchRows, dvportRows [][]string

	if slices.Contains(sheets, "dvSwitch") {
		dvswitchRows, err = excelFile.GetRows("dvSwitch")
		if err != nil {
			dvswitchRows = [][]string{}
		}
	}

	if slices.Contains(sheets, "dvPort") {
		dvportRows, err = excelFile.GetRows("dvPort")
		if err != nil {
			dvportRows = [][]string{}
		}
	}

	err = processNetworkInfo(dvswitchRows, dvportRows, &inventory)
	if err != nil {
		zap.S().Warnf("Network processing failed: %v", err)
		inventory.Infra.Networks = []api.Network{}
	}
	return &inventory, nil
}

func extractVCenterUUID(rows [][]string) (string, error) {
	if len(rows) < 2 {
		return "", fmt.Errorf("insufficient data")
	}

	header := rows[0]
	data := rows[1]

	for i, colName := range header {
		if colName == "VI SDK UUID" && i < len(data) {
			return data[i], nil
		}
	}

	return "", fmt.Errorf("VI SDK UUID column not found")
}

func IsExcelFile(content []byte) bool {
	if len(content) < 2 {
		return false
	}

	if content[0] == 0x50 && content[1] == 0x4B {
		f, err := excelize.OpenReader(bytes.NewReader(content))
		if err != nil {
			return false
		}
		defer f.Close()
		return true
	}

	return false
}

func resetHostInfo(inventory *api.Inventory) {
	inventory.Infra.TotalHosts = 0
	inventory.Infra.TotalClusters = 0
	inventory.Infra.HostsPerCluster = []int{}
	inventory.Infra.HostPowerStates = map[string]int{}
}
