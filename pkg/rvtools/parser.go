package rvtools

import (
	"bytes"
	"fmt"

	"github.com/xuri/excelize/v2"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
)

func ParseRVTools(rvtoolsContent []byte) (*api.Inventory, error) {
	excelFile, err := excelize.OpenReader(bytes.NewReader(rvtoolsContent))
	if err != nil {
		return nil, fmt.Errorf("error opening Excel file: %v", err)
	}
	defer excelFile.Close()
	
	sheets := excelFile.GetSheetList()
	
	inventory := api.Inventory{
		Vcenter: api.VCenter{},
		Infra: api.Infra{
			Datastores:      []api.Datastore{},
			HostPowerStates: map[string]int{},
			Networks: []struct {
				Dvswitch *string               `json:"dvswitch,omitempty"`
				Name     string                `json:"name"`
				Type     api.InfraNetworksType `json:"type"`
				VlanId   *string               `json:"vlanId,omitempty"`
			}{},
			HostsPerCluster: []int{},
		},
		Vms: api.VMs{
			PowerStates:          map[string]int{},
			Os:                   map[string]int{},
			MigrationWarnings:    api.MigrationIssues{},
			NotMigratableReasons: []struct {
				Assessment string `json:"assessment"`
				Count      int    `json:"count"`
				Label      string `json:"label"`
			}{},
		},
	}

	if contains(sheets, "vInfo") {
		vcenterUUID, err := extractVCenterUUID(excelFile, "vInfo")
		if err != nil {
			inventory.Vcenter.Id = ""
		} else {
			inventory.Vcenter.Id = vcenterUUID
		}
	} else {
		inventory.Vcenter.Id = ""
	}
	
	if contains(sheets, "vInfo") {
		vms, err := processVMInfo(excelFile, "vInfo")
		if err == nil && len(vms) > 0 {
			fillInventoryWithVMData(vms, &inventory)
		}
	}
	
	if contains(sheets, "vHost") {
		err := processHostInfo(excelFile, &inventory, "vHost")
		if err != nil {
			inventory.Infra.TotalHosts = 0
			inventory.Infra.TotalClusters = 0
			inventory.Infra.HostsPerCluster = []int{}
			inventory.Infra.HostPowerStates = map[string]int{}
		}
	}
	
	if contains(sheets, "vDatastore") {
		err := processDatastoreInfo(excelFile, &inventory, "vDatastore")
		if err != nil {
			inventory.Infra.Datastores = []api.Datastore{}
		}
	}
	
	err = processNetworkInfo(excelFile, &inventory)
	if err != nil {
		inventory.Infra.Networks = []struct {
			Dvswitch *string               `json:"dvswitch,omitempty"`
			Name     string                `json:"name"`
			Type     api.InfraNetworksType `json:"type"`
			VlanId   *string               `json:"vlanId,omitempty"`
		}{}
	}
	
	return &inventory, nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func extractVCenterUUID(excelFile *excelize.File, sheetName string) (string, error) {
    rows, err := excelFile.GetRows(sheetName)
    if err != nil || len(rows) < 2 {
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
		_, err := excelize.OpenReader(bytes.NewReader(content))
		return err == nil
	}
	
	return false
}