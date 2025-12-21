package rvtools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/kubev2v/migration-planner/pkg/opa"

	vsphere "github.com/kubev2v/forklift/pkg/controller/provider/model/vsphere"
	"github.com/xuri/excelize/v2"
	"go.uber.org/zap"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	collector "github.com/kubev2v/migration-planner/internal/agent/collector"
	"github.com/kubev2v/migration-planner/internal/agent/service"
)

// StatusCallback is called when the parsing status changes
type StatusCallback func(status string) error

// ParseRVTools parses an RVTools Excel export and returns inventory JSON.
// The optional statusCallback is called when transitioning to validation phase.
func ParseRVTools(ctx context.Context, rvtoolsContent []byte, opaValidator *opa.Validator, statusCallback StatusCallback) ([]byte, error) {
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

	if slices.Contains(sheets, "vInfo") {
		zap.S().Named("rvtools").Infof("Process VMs")

		// Read all VM-related sheets once
		datastoreRows := readSheet(excelFile, sheets, "vDatastore")
		datastoreMapping := make(map[string]string)
		if len(datastoreRows) > 0 {
			datastoreMapping = buildDatastoreMapping(datastoreRows)
		}

		vHostRows := readSheet(excelFile, sheets, "vHost")
		vCpuRows := readSheet(excelFile, sheets, "vCPU")
		vMemoryRows := readSheet(excelFile, sheets, "vMemory")
		vDiskRows := readSheet(excelFile, sheets, "vDisk")
		vNetworkRows := readSheet(excelFile, sheets, "vNetwork")
		dvPortRows := readSheet(excelFile, sheets, "dvPort")
		vClusterRows := readSheet(excelFile, sheets, "vCluster")
		dvswitchRows := readSheet(excelFile, sheets, "dvSwitch")

		vms, err := processVMInfo(vInfoRows, vCpuRows, vMemoryRows, vDiskRows, vNetworkRows, vHostRows, dvPortRows, datastoreMapping)
		if err != nil {
			zap.S().Named("rvtools").Warnf("VM processing failed: %v", err)
			vms = []vsphere.VM{}
		}

		if len(vms) > 0 {
			// Notify callback that we're transitioning to validation phase
			if statusCallback != nil {
				if err := statusCallback(string(api.Validating)); err != nil {
					zap.S().Named("rvtools").Warnf("Failed to update status to validating: %v", err)
				}
			}
			zap.S().Named("rvtools").Infof("Validating %d VMs using OPA validator", len(vms))
			if err := collector.ValidateVMs(ctx, opaValidator, vms); err != nil {
				zap.S().Named("rvtools").Warnf("At least one error during VMs validation: %v", err)
			}
		}

		zap.S().Named("rvtools").Infof("Process Hosts and Clusters")
		var hostPowerStates map[string]int
		var hosts []api.Host

		hostPowerStates = map[string]int{"green": 0}

		var clusterInfo ClusterInfo
		var hostIDToPowerState map[string]string
		if len(vHostRows) > 0 {
			clusterInfo = ExtractClusterAndDatacenterInfo(vHostRows)
			hostPowerStates = ExtractHostPowerStates(vHostRows)
			hostIDToPowerState = ExtractHostIDToPowerStateMap(vHostRows)

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
			hostIDToPowerState = make(map[string]string)
		}

		zap.S().Named("rvtools").Infof("Process Datastores")
		var datastores []api.Datastore
		var datastoreIndexToName map[int]string

		if len(datastoreRows) > 0 {
			tempInventory := &api.InventoryData{Infra: api.Infra{Datastores: []api.Datastore{}}}
			var err error
			datastoreIndexToName, err = processDatastoreInfo(datastoreRows, vHostRows, tempInventory)
			if err != nil {
				zap.S().Named("rvtools").Warnf("Failed to process datastore info: %v", err)
				datastores = []api.Datastore{}
				datastoreIndexToName = make(map[int]string)
			} else {
				multipathRows := readSheet(excelFile, sheets, "vMultiPath")
				hbaRows := readSheet(excelFile, sheets, "vHBA")

				correlateDatastoreInfo(multipathRows, hbaRows, tempInventory)
				datastores = tempInventory.Infra.Datastores
			}
		} else {
			datastores = []api.Datastore{}
			datastoreIndexToName = make(map[int]string)
		}

		zap.S().Named("rvtools").Infof("Process Networks")
		networks := ExtractNetworks(dvswitchRows, dvPortRows, vms)

		// Create vcenter-level aggregated inventory
		zap.S().Named("rvtools").Infof("Create Basic Inventory (VCenter Level)")
		infraData := service.InfrastructureData{
			Datastores:            datastores,
			Networks:              networks,
			HostPowerStates:       hostPowerStates,
			Hosts:                 &hosts,
			ClustersPerDatacenter: clusterInfo.ClustersPerDatacenter,
			TotalHosts:            clusterInfo.TotalHosts,
			TotalDatacenters:      clusterInfo.TotalDatacenters,
		}
		vcenterInventory := service.CreateBasicInventory(&vms, infraData)
		datastoreIDToType := buildDatastoreIDToTypeMapFromRVTools(datastoreRows)

		zap.S().Named("rvtools").Infof("Fill VCenter Inventory with VM Data")
		if len(vms) > 0 {
			collector.FillInventoryObjectWithMoreData(&vms, vcenterInventory, datastoreIDToType)
		}

		// Extract cluster ID mappings
		zap.S().Named("rvtools").Infof("Extract Cluster ID Mappings")
		clusterMapping := ExtractClusterIDMapping(vInfoRows, vHostRows, vClusterRows, vcenterUUID)
		zap.S().Named("rvtools").Infof("Found %d unique clusters", len(clusterMapping.ClusterIDs))

		// Build network ID to name mapping for filtering
		networkMapping := createNetworkMappings(dvPortRows).IDToName

		// Create per-cluster inventories
		zap.S().Named("rvtools").Infof("Create Per-Cluster Inventories")
		clusterInventories := make(map[string]api.InventoryData)

		for _, clusterID := range clusterMapping.ClusterIDs {
			zap.S().Named("rvtools").Infof("Processing cluster: %s", clusterID)

			clusterVMs := service.FilterVMsByClusterID(vms, clusterID, clusterMapping.VMToClusterID)
			zap.S().Named("rvtools").Infof("  - %d VMs in cluster %s", len(clusterVMs), clusterID)

			clusterInfraData := service.FilterInfraDataByClusterID(infraData, clusterID, clusterMapping.HostToClusterID, clusterVMs, datastoreMapping, datastoreIndexToName, networkMapping, hostIDToPowerState)

			clusterInv := service.CreateBasicInventory(&clusterVMs, clusterInfraData)

			if len(clusterVMs) > 0 {
				collector.FillInventoryObjectWithMoreData(&clusterVMs, clusterInv, datastoreIDToType)
			}

			clusterInventories[clusterID] = *clusterInv
		}

		inv := api.Inventory{
			VcenterId: vcenterUUID,
			Vcenter:   vcenterInventory,
			Clusters:  clusterInventories,
		}

		data, err := json.Marshal(inv)
		if err != nil {
			return []byte{}, err
		}

		return data, nil
	}

	// If vInfo doesn't exist, fail with error
	return nil, fmt.Errorf("vInfo sheet not found in RVTools export - cannot process inventory without VM data")
}

func buildDatastoreIDToTypeMapFromRVTools(datastoreRows [][]string) map[string]string {
	datastoreIDToType := make(map[string]string)

	if len(datastoreRows) <= 1 {
		return datastoreIDToType
	}

	colMap := buildColumnMap(datastoreRows[0])

	for _, row := range datastoreRows[1:] {
		if len(row) == 0 {
			continue
		}

		objectID := getColumnValue(row, colMap, "object id")
		dsType := getColumnValue(row, colMap, "type")

		if objectID != "" && dsType != "" {
			datastoreIDToType[objectID] = dsType
		}
	}

	return datastoreIDToType
}

type ClusterInfo struct {
	ClustersPerDatacenter []int
	TotalHosts            int
	TotalDatacenters      int
}

func ExtractClusterAndDatacenterInfo(vHostRows [][]string) ClusterInfo {
	if len(vHostRows) <= 1 {
		return ClusterInfo{}
	}

	colMap := buildColumnMap(vHostRows[0])

	hosts := make(map[string]struct{})
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
			datacenterToClusters[datacenter][cluster] = struct{}{}

			ensureMapExists(clusterToHosts, cluster)
			clusterToHosts[cluster][host] = struct{}{}
		}

	}

	return ClusterInfo{
		ClustersPerDatacenter: service.CalculateClustersPerDatacenter(datacenterToClusters),
		TotalHosts:            len(hosts),
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

func ExtractHostIDToPowerStateMap(rows [][]string) map[string]string {
	if len(rows) <= 1 {
		return map[string]string{}
	}

	colMap := buildColumnMap(rows[0])
	hostIDToPowerState := make(map[string]string)

	for _, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}

		hostID := getColumnValue(row, colMap, "object id")
		status := getColumnValue(row, colMap, "config status")

		if hostID == "" {
			continue
		}

		// Normalize status to valid values
		switch status {
		case "red", "yellow", "green", "gray":
			hostIDToPowerState[hostID] = status
		default:
			hostIDToPowerState[hostID] = "green"
		}
	}

	return hostIDToPowerState
}

func ExtractNetworks(dvswitchRows, dvportRows [][]string, vms []vsphere.VM) []api.Network {
	networks := []api.Network{}

	if len(dvswitchRows) == 0 && len(dvportRows) == 0 {
		zap.S().Named("rvtools").Infof("No network data available, returning empty networks array")
		return networks
	}

	tempInventory := &api.InventoryData{Infra: api.Infra{}}
	if err := processNetworkInfo(dvswitchRows, dvportRows, tempInventory, vms); err != nil {
		zap.S().Named("rvtools").Warnf("Failed to process network info: %v", err)
		return networks
	}

	networks = tempInventory.Infra.Networks
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
