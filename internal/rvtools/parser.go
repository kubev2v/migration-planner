package rvtools

import (
	"bytes"
	"context"
	"fmt"
	"slices"

	vsphere "github.com/kubev2v/forklift/pkg/controller/provider/model/vsphere"
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
		vcenterUUID, _ = ExtractVCenterUUID(vInfoRows)
	}

	datastoreRows := readSheet(excelFile, sheets, "vDatastore")
	datastoreMapping := make(map[string]string)
	if len(datastoreRows) > 0 {
		datastoreMapping = buildDatastoreMapping(datastoreRows)
	}

	zap.S().Named("rvtools").Infof("Process VMs")
	var vms []vsphere.VM
	if slices.Contains(sheets, "vInfo") {
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

	if len(vms) > 0 && opaValidator != nil {
		zap.S().Named("rvtools").Infof("Validating %d VMs using OPA validator", len(vms))
		validatedVms, err := opaValidator.ValidateVMs(ctx, vms)
		if err != nil {
			zap.S().Named("rvtools").Warnf("OPA validation failed, continuing without validation: %v", err)
		} else {
			vms = validatedVms
		}
	}

	zap.S().Named("rvtools").Infof("Process Hosts and Clusters")
	var hostPowerStates map[string]int
	var hosts []api.Host

	hostPowerStates = map[string]int{"green": 0}

	vHostRows := readSheet(excelFile, sheets, "vHost")
	var clusterInfo ClusterInfo
	if len(vHostRows) > 0 {
		clusterInfo = ExtractClusterAndDatacenterInfo(vHostRows)
		hostPowerStates = ExtractHostPowerStates(vHostRows)

		var err error
		hosts, err = ExtractHostsInfo(vHostRows)
		if err != nil {
			zap.S().Named("rvtools").Warnf("Failed to extract host info: %v", err)
			hosts = []api.Host{}
		}
	} else {
		zap.S().Named("rvtools").Infof("vHost sheet not found, using default values")
		clusterInfo = ClusterInfo{}
		hosts = []api.Host{}
	}

	zap.S().Named("rvtools").Infof("Process Datastores")
	var datastores []api.Datastore

	if len(datastoreRows) > 0 {
		tempInventory := &api.Inventory{Infra: api.Infra{Datastores: []api.Datastore{}}}
		vHostRows := readSheet(excelFile, sheets, "vHost")
		err := processDatastoreInfo(datastoreRows, vHostRows, tempInventory)
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

	networks := ExtractNetworks(dvswitchRows, dvportRows)

	zap.S().Named("rvtools").Infof("Create Basic Inventory")
	infraData := service.InfrastructureData{
		Datastores:            datastores,
		Networks:              networks,
		HostPowerStates:       hostPowerStates,
		Hosts:                 &hosts,
		HostsPerCluster:       clusterInfo.HostsPerCluster,
		ClustersPerDatacenter: clusterInfo.ClustersPerDatacenter,
		TotalHosts:            clusterInfo.TotalHosts,
		TotalClusters:         clusterInfo.TotalClusters,
		TotalDatacenters:      clusterInfo.TotalDatacenters,
		VmsPerCluster:         ExtractVmsPerCluster(vInfoRows),
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

func ExtractClusterAndDatacenterInfo(vHostRows [][]string) ClusterInfo {
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
		if !hasValue(host) {
			continue
		}

		datacenter := getColumnValue(row, colMap, "datacenter")
		cluster := getColumnValue(row, colMap, "cluster")

		hosts[host] = struct{}{}

		if hasValue(datacenter) {
			datacenters[datacenter] = struct{}{}
			ensureMapExists(datacenterToClusters, datacenter)
		}

		if hasValue(datacenter) && hasValue(cluster) {
			clusters[cluster] = struct{}{}
			datacenterToClusters[datacenter][cluster] = struct{}{}

			ensureMapExists(clusterToHosts, cluster)
			clusterToHosts[cluster][host] = struct{}{}
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

func ExtractHostsInfo(vHostRows [][]string) ([]api.Host, error) {
	if len(vHostRows) <= 1 {
		return []api.Host{}, nil
	}

	colMap := buildColumnMap(vHostRows[0])

	// Validate required columns exist
	requiredCols := []string{"vendor", "model", "object id"}
	for _, col := range requiredCols {
		if _, exists := colMap[col]; !exists {
			return nil, fmt.Errorf("missing required column: %s", col)
		}
	}

	var hosts []api.Host
	for _, row := range vHostRows[1:] {
		if len(row) == 0 {
			continue
		}

		host := api.Host{
			Id:     stringPtrIfNotEmpty(getColumnValue(row, colMap, "object id")),
			Vendor: getColumnValue(row, colMap, "vendor"),
			Model:  getColumnValue(row, colMap, "model"),
		}

		host.CpuCores = parseIntPtr(getColumnValue(row, colMap, "# cores"))
		host.CpuSockets = parseIntPtr(getColumnValue(row, colMap, "# cpu"))

		if memStr := getColumnValue(row, colMap, "# memory"); memStr != "" {
			if memMB := parseMemoryMB(memStr); memMB > 0 {
				mem := int64(memMB)
				host.MemoryMB = &mem
			}
		}

		hosts = append(hosts, host)
	}

	return hosts, nil
}

func ExtractHostPowerStates(rows [][]string) map[string]int {
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

func ExtractVmsPerCluster(rows [][]string) []int {
	if len(rows) <= 1 {
		return []int{}
	}

	colMap := buildColumnMap(rows[0])
	clusterToVMs := make(map[string]map[string]struct{})

	for _, row := range rows[1:] {
		cluster := getColumnValue(row, colMap, "cluster")
		vm := getColumnValue(row, colMap, "vm")

		if hasValue(cluster) && hasValue(vm) {
			ensureMapExists(clusterToVMs, cluster)
			clusterToVMs[cluster][vm] = struct{}{}
		}
	}

	return calculateVMsPerCluster(clusterToVMs)
}

func ExtractNetworks(dvswitchRows, dvportRows [][]string) []api.Network {
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

func ExtractVCenterUUID(rows [][]string) (string, error) {
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
