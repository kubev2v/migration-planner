package rvtools

import (
	"bytes"
	"context"
	"fmt"
	"slices"

	vsphere "github.com/kubev2v/forklift/pkg/controller/provider/model/vsphere"
	vspheremodel "github.com/kubev2v/forklift/pkg/controller/provider/model/vsphere"
	web "github.com/kubev2v/forklift/pkg/controller/provider/web/vsphere"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	collector "github.com/kubev2v/migration-planner/internal/agent/collector"
	"github.com/kubev2v/migration-planner/internal/agent/service"
	"github.com/kubev2v/migration-planner/internal/opa"
	"github.com/xuri/excelize/v2"
	"go.uber.org/zap"
)

func ParseRVTools(ctx context.Context, rvtoolsContent []byte, opaValidator *opa.Validator) (*api.Inventory, error) {
	excelFile, err := excelize.OpenReader(bytes.NewReader(rvtoolsContent))
	if err != nil {
		return nil, fmt.Errorf("error opening Excel file: %v", err)
	}
	defer excelFile.Close()

	sheets := excelFile.GetSheetList()

	var vcenterUUID string
	vInfoRows := readSheet(excelFile, sheets, "vInfo")
	if len(vInfoRows) > 1 {
		vcenterUUID, _ = extractVCenterUUID(vInfoRows)
	}

	datastoreRows := readSheet(excelFile, sheets, "vDatastore")
	datastoreMapping := make(map[string]string)
	if len(datastoreRows) > 0 {
		datastoreMapping = buildDatastoreMapping(datastoreRows)
	}

	zap.S().Named("rvtools").Infof("Process VMs")
	var vms []vsphere.VM
	if slices.Contains(sheets, "vInfo") {
		vInfoRows := readSheet(excelFile, sheets, "vInfo")
		vHostRows := readSheet(excelFile, sheets, "vHost")
		vCpuRows := readSheet(excelFile, sheets, "vCPU")
		vMemoryRows := readSheet(excelFile, sheets, "vMemory")
		vDiskRows := readSheet(excelFile, sheets, "vDisk")
		vNetworkRows := readSheet(excelFile, sheets, "vNetwork")
		dvPortRows := readSheet(excelFile, sheets, "dvPort")

		vms, err = processVMInfo(vInfoRows, vCpuRows, vMemoryRows, vDiskRows, vNetworkRows, vHostRows, dvPortRows, datastoreMapping)
		if err != nil {
			zap.S().Named("rvtools").Warnf("VM processing failed: %v", err)
			vms = []vsphere.VM{}
		}
	}

	zap.S().Named("rvtools").Infof("Validate VMs against OPA")
	if len(vms) > 0 {
		vms, err = validateVMsWithOPA(ctx, vms, opaValidator)
		if err != nil {
			zap.S().Named("rvtools").Warnf("OPA validation failed, continuing without validation: %v", err)
		}
	}

	zap.S().Named("rvtools").Infof("Process Hosts and Clusters")
	var hostPowerStates map[string]int

	hostPowerStates = map[string]int{"green": 0}

	vHostRows := readSheet(excelFile, sheets, "vHost")
	var clusterInfo ClusterInfo
	if len(vHostRows) > 0 {
		clusterInfo = extractClusterAndDatacenterInfo(vHostRows)
		hostPowerStates = extractHostPowerStates(vHostRows)
	} else {
		zap.S().Named("rvtools").Infof("vHost sheet not found, using default values")
		clusterInfo = ClusterInfo{}
	}

	zap.S().Named("rvtools").Infof("Process Datastores")
	var datastores []api.Datastore

	if len(datastoreRows) > 0 {
		tempInventory := &api.Inventory{Infra: api.Infra{Datastores: []api.Datastore{}}}
		err := processDatastoreInfo(datastoreRows, tempInventory)
		if err != nil {
			zap.S().Named("rvtools").Warnf("Failed to process datastore info: %v", err)
			datastores = []api.Datastore{}
		} else {
			multipathRows := readSheet(excelFile, sheets, "vMultiPath")
			hbaRows := readSheet(excelFile, sheets, "vHBA")

			correlateDatastoreInfo(multipathRows, hbaRows, tempInventory)
			datastores = tempInventory.Infra.Datastores
		}
	} else {
		datastores = []api.Datastore{}
	}

	zap.S().Named("rvtools").Infof("Process Networks")

	dvswitchRows := readSheet(excelFile, sheets, "dvSwitch")
	dvportRows := readSheet(excelFile, sheets, "dvPort")

	networks := extractNetworks(dvswitchRows, dvportRows)

	zap.S().Named("rvtools").Infof("Create Basic Inventory")
	infraData := service.InfrastructureData{
		Datastores:            datastores,
		Networks:              networks,
		HostPowerStates:       hostPowerStates,
		Hosts:                 &[]api.Host{}, // RVTools doesn't provide detailed host info
		HostsPerCluster:       clusterInfo.HostsPerCluster,
		ClustersPerDatacenter: clusterInfo.ClustersPerDatacenter,
		TotalHosts:            clusterInfo.TotalHosts,
		TotalClusters:         clusterInfo.TotalClusters,
		TotalDatacenters:      clusterInfo.TotalDatacenters,
	}
	inventory := service.CreateBasicInventory(vcenterUUID, &vms, infraData)

	zap.S().Named("rvtools").Infof("Fill Inventory with VM Data")
	if len(vms) > 0 {
		collector.FillInventoryObjectWithMoreData(&vms, inventory)
	}

	return inventory, nil
}

type ClusterInfo struct {
	HostsPerCluster       []int
	ClustersPerDatacenter []int
	TotalHosts            int
	TotalClusters         int
	TotalDatacenters      int
}

func extractClusterAndDatacenterInfo(vHostRows [][]string) ClusterInfo {
	if len(vHostRows) <= 1 {
		return ClusterInfo{}
	}

	colMap := buildColumnMap(vHostRows[0])

	hosts := make(map[string]struct{})
	clusters := make(map[string]struct{})
	datacenters := make(map[string]struct{})

	datacenterToClusters := make(map[string]map[string]struct{})
	clusterToHosts := make(map[string]map[string]struct{})

	for _, row := range vHostRows[1:] {
		if len(row) == 0 {
			continue
		}

		host := getColumnValue(row, colMap, "host")
		if host == "" {
			continue
		}

		datacenter := getColumnValue(row, colMap, "datacenter")
		cluster := getColumnValue(row, colMap, "cluster")

		hosts[host] = struct{}{}

		if datacenter != "" {
			datacenters[datacenter] = struct{}{}
			ensureMapExists(datacenterToClusters, datacenter)

			if cluster != "" {
				clusters[cluster] = struct{}{}
				datacenterToClusters[datacenter][cluster] = struct{}{}

				ensureMapExists(clusterToHosts, cluster)
				clusterToHosts[cluster][host] = struct{}{}
			}
		}
	}

	return ClusterInfo{
		HostsPerCluster:       calculateHostsPerCluster(clusterToHosts),
		ClustersPerDatacenter: calculateClustersPerDatacenter(datacenterToClusters),
		TotalHosts:            len(hosts),
		TotalClusters:         len(clusters),
		TotalDatacenters:      len(datacenters),
	}
}

func extractHostPowerStates(rows [][]string) map[string]int {
	if len(rows) <= 1 {
		return map[string]int{}
	}

	colMap := buildColumnMap(rows[0])
	hostPowerStates := map[string]int{}

	for _, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}

		status := getColumnValue(row, colMap, "config status")

		switch status {
		case "red", "yellow", "green", "gray":
			hostPowerStates[status]++
		default:
			hostPowerStates["green"]++
		}
	}

	return hostPowerStates
}

func extractNetworks(dvswitchRows, dvportRows [][]string) []api.Network {
	networks := []api.Network{}

	if len(dvswitchRows) == 0 && len(dvportRows) == 0 {
		zap.S().Named("rvtools").Infof("No network data available, returning empty networks array")
		return networks
	}

	tempInventory := &api.Inventory{Infra: api.Infra{}}
	if err := processNetworkInfo(dvswitchRows, dvportRows, tempInventory); err == nil {
		networks = tempInventory.Infra.Networks
	}

	return networks
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

func validateVMsWithOPA(ctx context.Context, vms []vsphere.VM, opaValidator *opa.Validator) ([]vsphere.VM, error) {
	if opaValidator == nil {
		zap.S().Named("rvtools").Warn("OPA validator not available, skipping validation")
		return vms, nil
	}

	zap.S().Named("rvtools").Infof("Validating %d VMs using OPA validator", len(vms))

	validatedVMs := make([]vsphere.VM, 0, len(vms))

	for _, vm := range vms {
		// Prepare the JSON data in MTV OPA server format
		workload := web.Workload{}
		workload.With(&vm)

		concerns, err := opaValidator.ValidateConcerns(ctx, workload)
		if err != nil {
			zap.S().Named("rvtools").Warnf("Failed to evaluate VM %s: %v", vm.Name, err)
			validatedVMs = append(validatedVMs, vm)
			continue
		}

		// Convert concerns to vsphere.Concern format
		for _, concernData := range concerns {
			concernMap, ok := concernData.(map[string]interface{})
			if !ok {
				zap.S().Named("rvtools").Warnf("Unexpected concern data type for VM %s", vm.Name)
				continue
			}

			concern := vspheremodel.Concern{}
			if id, ok := concernMap["id"].(string); ok {
				concern.Id = id
			} else {
				zap.S().Named("rvtools").Warnf("Missing or invalid 'id' field in concern for VM %s", vm.Name)
			}

			if label, ok := concernMap["label"].(string); ok {
				concern.Label = label
			} else {
				zap.S().Named("rvtools").Warnf("Missing or invalid 'label' field in concern for VM %s", vm.Name)
			}

			if assessment, ok := concernMap["assessment"].(string); ok {
				concern.Assessment = assessment
			}

			if category, ok := concernMap["category"].(string); ok {
				concern.Category = category
			}

			vm.Concerns = append(vm.Concerns, concern)
		}

		validatedVMs = append(validatedVMs, vm)
	}

	zap.S().Named("rvtools").Infof("Successfully validated %d VMs", len(validatedVMs))
	return validatedVMs, nil
}
