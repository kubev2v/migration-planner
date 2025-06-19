package rvtools

import (
	"bytes"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	vsphere "github.com/konveyor/forklift-controller/pkg/controller/provider/model/vsphere"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	collector "github.com/kubev2v/migration-planner/internal/agent/collector"
	"github.com/kubev2v/migration-planner/internal/util"
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

	var vcenterUUID string
	if slices.Contains(sheets, "vInfo") {
		vInfoRows, err := excelFile.GetRows("vInfo")
		if err == nil && len(vInfoRows) > 1 {
			vcenterUUID, _ = extractVCenterUUID(vInfoRows)
		}
	}

	var datastoreMapping map[string]string
	if slices.Contains(sheets, "vDatastore") {
		datastoreRows, err := excelFile.GetRows("vDatastore")
		if err != nil {
			zap.S().Named("rvtools").Warnf("Could not read vDatastore sheet for mapping: %v", err)
			datastoreMapping = make(map[string]string)
		} else {
			datastoreMapping = buildDatastoreMapping(datastoreRows)
		}
	} else {
		datastoreMapping = make(map[string]string)
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
		validatedVMs, err := validateVMsWithOPA(vms)
		if err != nil {
			zap.S().Named("rvtools").Warnf("OPA validation failed, continuing without validation: %v", err)
		} else {
			vms = validatedVMs
		}
	}

	zap.S().Named("rvtools").Infof("Process Hosts and Clusters")
	var hostPowerStates map[string]int
	var hostsPerCluster []int
	var clustersPerDatacenter []int
	var totalHosts, totalClusters, totalDatacenters int

	hostPowerStates = map[string]int{"green": 0}
	hostsPerCluster = []int{0}
	clustersPerDatacenter = []int{0}
	totalHosts, totalClusters, totalDatacenters = 0, 0, 0

	if slices.Contains(sheets, "vHost") {
		if rows, err := excelFile.GetRows("vHost"); err != nil {
			zap.S().Named("rvtools").Warnf("Could not read vHost sheet: %v", err)
		} else {
			hostsPerCluster, clustersPerDatacenter, totalHosts, totalClusters, totalDatacenters = extractClusterAndDatacenterInfo(rows)
			_, hostPowerStates, _ = extractHostInfo(rows)
		}
	} else {
		zap.S().Named("rvtools").Infof("vHost sheet not found, using default values")
	}


	zap.S().Named("rvtools").Infof("Process Datastores")

	datastoreRows := readSheet(excelFile, sheets, "vDatastore")
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
	inventory := createBasicInventoryObj(
		vcenterUUID, 
		&vms, 
		datastores, 
		networks,
		hostPowerStates, 
		hostsPerCluster, 
		clustersPerDatacenter,
		totalHosts, 
		totalClusters, 
		totalDatacenters,
	)

	zap.S().Named("rvtools").Infof("Fill Inventory with VM Data")
	if len(vms) > 0 {
		collector.FillInventoryObjectWithMoreData(&vms, inventory)
	}

	return inventory, nil
}

func createBasicInventoryObj(
	vCenterID string,
	vms *[]vsphere.VM,
	datastores []api.Datastore,
	networks []api.Network,
	hostPowerStates map[string]int,
	hostsPerCluster []int,
	clustersPerDatacenter []int,
	totalHosts, totalClusters, totalDatacenters int,
) *api.Inventory {
	return &api.Inventory{
		Vcenter: api.VCenter{
			Id: vCenterID,
		},
		Vms: api.VMs{
			Total:       len(*vms),
			PowerStates: map[string]int{},
			Os:          map[string]int{},
			MigrationWarnings:    api.MigrationIssues{},
			NotMigratableReasons: api.MigrationIssues{},
		},
		Infra: api.Infra{
			ClustersPerDatacenter: clustersPerDatacenter,
			Datastores:            datastores,
			HostPowerStates:       hostPowerStates,
			TotalHosts:            totalHosts,
			TotalClusters:         totalClusters,
			TotalDatacenters:      totalDatacenters,
			HostsPerCluster:       hostsPerCluster,
			Networks:              networks,
		},
	}
}

func extractClusterAndDatacenterInfo(vHostRows [][]string) ([]int, []int, int, int, int) {
	if len(vHostRows) <= 1 {
		return []int{}, []int{}, 0, 0, 0
	}

	headers := vHostRows[0]
	colMap := make(map[string]int)
	for i, header := range headers {
		key := strings.ToLower(strings.TrimSpace(header))
		colMap[key] = i
	}

	uniqueHosts := make(map[string]struct{})
	uniqueClusters := make(map[string]struct{})
	uniqueDatacenters := make(map[string]struct{})

	datacenterToCluster := make(map[string]map[string]struct{})
	clusterToHosts := make(map[string]map[string]struct{})

	for i := 1; i < len(vHostRows); i++ {
		row := vHostRows[i]
		if len(row) == 0 {
			continue
		}

		host := getColumnValue(row, colMap, "host")
		datacenter := getColumnValue(row, colMap, "datacenter")
		cluster := getColumnValue(row, colMap, "cluster")

		if host == "" {
			continue
		}

		uniqueHosts[host] = struct{}{}

		if datacenter != "" {
			uniqueDatacenters[datacenter] = struct{}{}

			if _, ok := datacenterToCluster[datacenter]; !ok {
				datacenterToCluster[datacenter] = make(map[string]struct{})
			}

			if cluster != "" {
				uniqueClusters[cluster] = struct{}{}
				datacenterToCluster[datacenter][cluster] = struct{}{}

				if _, ok := clusterToHosts[cluster]; !ok {
					clusterToHosts[cluster] = make(map[string]struct{})
				}
				clusterToHosts[cluster][host] = struct{}{}
			}
		}
	}

	totalHosts := len(uniqueHosts)
	totalClusters := len(uniqueClusters)
	totalDatacenters := len(uniqueDatacenters)

	var hostsPerCluster []int
	var clusterKeys []string
	for cluster := range uniqueClusters {
		clusterKeys = append(clusterKeys, cluster)
	}
	sort.Strings(clusterKeys)
	for _, cluster := range clusterKeys {
		hostCount := len(clusterToHosts[cluster])
		hostsPerCluster = append(hostsPerCluster, hostCount)
	}

	var clustersPerDatacenter []int
	for datacenter := range uniqueDatacenters {
		clusterCount := len(datacenterToCluster[datacenter])
		clustersPerDatacenter = append(clustersPerDatacenter, clusterCount)
	}

	return hostsPerCluster, clustersPerDatacenter, totalHosts, totalClusters, totalDatacenters
}

func extractHostInfo(rows [][]string) ([]vsphere.Host, map[string]int, int) {
	if len(rows) <= 1 {
		return nil, map[string]int{}, 0
	}

	colMap := buildColumnMap(rows[0])
	hosts := make([]vsphere.Host, 0, len(rows)-1)
	hostPowerStates := map[string]int{}

	for _, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}

		id := getColumnValue(row, colMap, "object id")
		name := getColumnValue(row, colMap, "host")
		status := getColumnValue(row, colMap, "config status")

		if id == "" {
			id = name // fallback
		}

		host := vsphere.Host{
			Base: vsphere.Base{
				ID:   id,
				Name: name,
			},
			Status: status,
		}
		hosts = append(hosts, host)

		switch status {
		case "red", "yellow", "green", "gray":
			hostPowerStates[status]++
		default:
			hostPowerStates["green"]++
		}
	}

	return hosts, hostPowerStates, len(hosts)
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

func validateVMsWithOPA(vms []vsphere.VM) ([]vsphere.VM, error) {
	opaServer := util.GetEnv("OPA_SERVER", "127.0.0.1:8181")

	if !isOPAServerAlive(opaServer) {
		return vms, fmt.Errorf("OPA server %s is not responding", opaServer)
	}

	zap.S().Named("rvtools").Infof("OPA server %s is alive, validating VMs", opaServer)

	validatedVMs, err := collector.Validation(&vms, opaServer)
	if err != nil {
		return vms, err
	}

	return *validatedVMs, nil
}

func isOPAServerAlive(opaServer string) bool {
	zap.S().Named("rvtools").Infof("Check if OPA server is responding")
	
	client := &http.Client{
		Timeout: 5 * time.Second, // 5 second timeout
	}
	
	resp, err := client.Get("http://" + opaServer + "/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		zap.S().Named("rvtools").Errorf("OPA server %s is not responding", opaServer)
		return false
	}
	defer resp.Body.Close()
	zap.S().Named("rvtools").Infof("OPA server %s is alive", opaServer)
	return true
}