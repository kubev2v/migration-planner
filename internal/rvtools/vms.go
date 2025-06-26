package rvtools

import (
	"regexp"
	"sort"
	"strings"

	vsphere "github.com/kubev2v/forklift/pkg/controller/provider/model/vsphere"
)

func processVMInfo(
	vInfoRows, vCpuRows, vMemoryRows, vDiskRows, vNetworkRows, vHostRows, dvPortRows [][]string,
	datastoreMapping map[string]string,
) ([]vsphere.VM, error) {
	if len(vInfoRows) <= 1 {
		return nil, nil
	}

	vCpuHeader, vCpuData := splitSheet(vCpuRows)
	vMemoryHeader, vMemoryData := splitSheet(vMemoryRows)
	vDiskHeader, vDiskData := splitSheet(vDiskRows)
	vNetworkHeader, vNetworkData := splitSheet(vNetworkRows)

	vInfoColMap := buildColumnMap(vInfoRows[0])
	vCpuColMap := buildColumnMap(vCpuHeader)
	vMemoryColMap := buildColumnMap(vMemoryHeader)
	vDiskColMap := buildColumnMap(vDiskHeader)
	vNetworkColMap := buildColumnMap(vNetworkHeader)

	hostIPToObjectID := createHostIPToObjectIDMap(vHostRows)
	networkNameToID := createNetworkNameToIDMap(dvPortRows)

	vmCpuData := groupRowsByVM(vCpuData, vCpuColMap)
	vmMemoryData := groupRowsByVM(vMemoryData, vMemoryColMap)
	vmDiskData := groupRowsByVM(vDiskData, vDiskColMap)
	vmNetworkData := groupRowsByVM(vNetworkData, vNetworkColMap)

	vms := make([]vsphere.VM, 0, len(vInfoRows)-1)

	for _, row := range vInfoRows[1:] {
		if len(row) == 0 {
			continue
		}

		vmName := getColumnValue(row, vInfoColMap, "vm")
		if vmName == "" {
			continue
		}

		vm := vsphere.VM{}
		populateVMInfoData(&vm, row, vInfoColMap, hostIPToObjectID)

		if cpuRows, exists := vmCpuData[vmName]; exists && len(cpuRows) > 0 {
			populateVMCpuData(&vm, cpuRows[0], vCpuColMap)
		}

		if memRows, exists := vmMemoryData[vmName]; exists && len(memRows) > 0 {
			populateVMMemoryData(&vm, memRows[0], vMemoryColMap)
		}

		if diskRows, exists := vmDiskData[vmName]; exists && len(diskRows) > 0 {
			vm.Disks = processVMDisksFromDiskSheet(diskRows, vDiskColMap, datastoreMapping)
		}

		if nicRows, exists := vmNetworkData[vmName]; exists && len(nicRows) > 0 {
			vm.NICs = processVMNICs(nicRows, vNetworkColMap, networkNameToID)
		}

		vm.Networks = processVMNetworksFromInfo(row, vInfoColMap, networkNameToID)
		vms = append(vms, vm)
	}

	return vms, nil
}


func populateVMInfoData(vm *vsphere.VM, row []string, colMap map[string]int, hostIPToObjectID map[string]string) {

	vm.ID = getColumnValue(row, colMap, "vm id")
	vm.Name = getColumnValue(row, colMap, "vm")
	vm.Folder = getColumnValue(row, colMap, "folder id")
	
	// Resolve host IP to Object ID
	hostIP := getColumnValue(row, colMap, "host")
	if objectID, exists := hostIPToObjectID[hostIP]; exists {
		vm.Host = objectID
	}
	
	vm.UUID = getColumnValue(row, colMap, "smbios uuid")
	vm.Firmware = getColumnValue(row, colMap, "firmware")
	vm.PowerState = getColumnValue(row, colMap, "powerstate")
	vm.ConnectionState = getColumnValue(row, colMap, "connection state")
	vm.CpuCount = parseIntOrZero(getColumnValue(row, colMap, "cpus"))
	vm.MemoryMB = parseMemoryMB(getColumnValue(row, colMap, "memory"))
	vm.GuestName = getColumnValue(row, colMap, "os according to the configuration file")
	vm.GuestNameFromVmwareTools = getColumnValue(row, colMap, "os according to the vmware tools")
	vm.HostName = getColumnValue(row, colMap, "dns name")
	vm.IpAddress = getColumnValue(row, colMap, "primary ip address")
	vm.IsTemplate = parseBooleanValue(getColumnValue(row, colMap, "template"))
	vm.ChangeTrackingEnabled = parseBooleanValue(getColumnValue(row, colMap, "cbt"))
	vm.DiskEnableUuid = parseBooleanValue(getColumnValue(row, colMap, "enableuuid"))
	vm.StorageUsed = parseFormattedInt64(getColumnValue(row, colMap, "in use mib")) * 1024 * 1024 // Convert MiB to bytes

	ftState := getColumnValue(row, colMap, "ft state")
	vm.FaultToleranceEnabled = ftState != "" && ftState != "notConfigured"
}

func populateVMCpuData(vm *vsphere.VM, cpuRow []string, colMap map[string]int) {
	vm.CpuHotAddEnabled = parseBooleanValue(getColumnValue(cpuRow, colMap, "hot add"))
	vm.CpuHotRemoveEnabled = parseBooleanValue(getColumnValue(cpuRow, colMap, "hot remove"))
	vm.CoresPerSocket = parseIntOrZero(getColumnValue(cpuRow, colMap, "cores p/s"))
}

func populateVMMemoryData(vm *vsphere.VM, memRow []string, colMap map[string]int) {
	vm.MemoryHotAddEnabled = parseBooleanValue(getColumnValue(memRow, colMap, "hot add"))
	vm.BalloonedMemory = parseIntOrZero(getColumnValue(memRow, colMap, "ballooned"))
}

type ControllerTracker struct {
	ideCount   int32
	scsiCount  int32
	sataCount  int32
	nvmeCount  int32
}

func NewControllerTracker() *ControllerTracker {
	return &ControllerTracker{}
}

func processVMDisksFromDiskSheet(diskRows [][]string, colMap map[string]int, datastoreMapping map[string]string) []vsphere.Disk {
	disks := []vsphere.Disk{}

	controllerTracker := NewControllerTracker()
	controllerKeyMap := make(map[string]int32)
	
	for _, diskRow := range diskRows {
		capacityMiB := parseFormattedInt64(getColumnValue(diskRow, colMap, "capacity mib"))
		disk := vsphere.Disk{
			Key:           parseIntOrZero(getColumnValue(diskRow, colMap, "disk key")),
			UnitNumber:    parseIntOrZero(getColumnValue(diskRow, colMap, "unit #")),
			File:          getColumnValue(diskRow, colMap, "path"),
			Capacity:      capacityMiB * 1024 * 1024,
			Mode:          getColumnValue(diskRow, colMap, "disk mode"),
			Serial:        getColumnValue(diskRow, colMap, "disk uuid"),
		}

		// Parse datastore from disk path and map to object ID
		if disk.File != "" {
			datastoreName := parseDatastoreFromPath(disk.File)
			if datastoreName != "" {
				if objectID, exists := datastoreMapping[datastoreName]; exists {
					disk.Datastore = vsphere.Ref{
						Kind: "Datastore",
						ID:   objectID,
					}
				}
			}
		}

		sharingMode := getColumnValue(diskRow, colMap, "sharing mode")
		disk.Shared = sharingMode != "" && sharingMode != "sharingNone"

		rawValue := getColumnValue(diskRow, colMap, "raw")
		disk.RDM = rawValue != "" && rawValue != "0" && strings.ToLower(rawValue) != "false"

		controllerName := getColumnValue(diskRow, colMap, "controller")

		if existingKey, exists := controllerKeyMap[controllerName]; exists {
			disk.ControllerKey = existingKey
		} else {
			newKey := controllerTracker.GetControllerKey(controllerName)
			controllerKeyMap[controllerName] = newKey
			disk.ControllerKey = newKey
		}
		
		disk.Bus = mapControllerNameToBus(controllerName)

		disks = append(disks, disk)
	}
	
	return disks
}

func createHostIPToObjectIDMap(vHostRows [][]string) map[string]string {
	hostMap := make(map[string]string)
	
	if len(vHostRows) <= 1 {
		return hostMap
	}

	vHostColMap := buildColumnMap(vHostRows[0])

	for i := 1; i < len(vHostRows); i++ {
		row := vHostRows[i]
		if len(row) == 0 {
			continue
		}
		
		hostIP := getColumnValue(row, vHostColMap, "host")
		objectID := getColumnValue(row, vHostColMap, "object id")
		
		if hostIP != "" && objectID != "" {
			hostMap[hostIP] = objectID
		}
	}
	
	return hostMap
}

func processVMNICs(nicRows [][]string, colMap map[string]int, networkNameToID map[string]string) []vsphere.NIC {
	nics := []vsphere.NIC{}

	for _, nicRow := range nicRows {
		networkName := getColumnValue(nicRow, colMap, "network")
		
		nic := vsphere.NIC{
			MAC:       getColumnValue(nicRow, colMap, "mac address"),
		}

		if networkName != "" {
			if objectID, exists := networkNameToID[networkName]; exists {
				nic.Network = vsphere.Ref{
					Kind: "Network",
					ID:   objectID,
				}
			}
		}

		nics = append(nics, nic)
	}

	return nics
}

func processVMNetworksFromInfo(row []string, colMap map[string]int, networkNameToID map[string]string) []vsphere.Ref {
	networks := []vsphere.Ref{}
	
	// Check for Network #1, Network #2, etc. columns
	// Dynamically find all 'network #<number>' columns
	networkKeys := []string{}
	networkKeyPattern := regexp.MustCompile(`(?i)^network #\d+$`)
	for key := range colMap {
		if networkKeyPattern.MatchString(key) {
			networkKeys = append(networkKeys, key)
		}
	}
	sort.Strings(networkKeys)

	for _, networkKey := range networkKeys {
		networkName := getColumnValue(row, colMap, networkKey)
		if networkName != "" {
			if objectID, exists := networkNameToID[networkName]; exists {
				networks = append(networks, vsphere.Ref{
					Kind: "Network",
					ID:   objectID,
				})
			}
		}
	}
	
	return networks
}

