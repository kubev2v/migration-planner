package rvtools

import (
	"strconv"
	"strings"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/agent/service"
)

type VM struct {
	Name       string
	PowerState string
	OS         string
	CPUCount   int
	MemoryMB   int
	Disks      []Disk
	CBTEnabled bool
}

type Disk struct {
	Name     string
	Capacity int
}

func processVMInfo(rows [][]string) ([]VM, error) {
	if len(rows) <= 1 {
		return []VM{}, nil
	}

	colMap := make(map[string]int)
	for i, header := range rows[0] {
		key := strings.ToLower(strings.TrimSpace(header))
		colMap[key] = i
	}

	templateIdx := -1
	if idx, exists := colMap["template"]; exists {
		templateIdx = idx
	}

	cbtIdx := -1
	if idx, exists := colMap["cbt"]; exists {
		cbtIdx = idx
	}

	vms := []VM{}

	for i := 1; i < len(rows); i++ {
		row := rows[i]
		if len(row) == 0 {
			continue
		}

		if templateIdx >= 0 && templateIdx < len(row) {
			if strings.ToLower(strings.TrimSpace(row[templateIdx])) == "true" {
				continue
			}
		}

		vm := VM{}

		if idx, exists := colMap["vm"]; exists && idx < len(row) {
			vm.Name = row[idx]
		}

		if vm.Name == "" {
			continue
		}

		if idx, exists := colMap["powerstate"]; exists && idx < len(row) {
			vm.PowerState = row[idx]
		}

		if idx, exists := colMap["os according to the VMware Tools"]; exists && idx < len(row) && row[idx] != "" {
			vm.OS = row[idx]
		} else if idx, exists := colMap["os according to the configuration file"]; exists && idx < len(row) && row[idx] != "" {
			vm.OS = row[idx]
		}

		if idx, exists := colMap["cpus"]; exists && idx < len(row) {
			if cpuCount, err := strconv.Atoi(strings.TrimSpace(row[idx])); err == nil && cpuCount > 0 {
				vm.CPUCount = cpuCount
			}
		}

		if idx, exists := colMap["memory"]; exists && idx < len(row) {
			memStr := strings.Map(func(r rune) rune {
				if (r >= '0' && r <= '9') || r == '.' {
					return r
				}
				return -1
			}, row[idx])

			if memVal, err := strconv.ParseFloat(memStr, 64); err == nil {
				if memVal < 100 {
					vm.MemoryMB = int(memVal * 1024)
				} else {
					vm.MemoryMB = int(memVal)
				}
			}
		}

		if cbtIdx >= 0 && cbtIdx < len(row) {
			vm.CBTEnabled = strings.ToLower(strings.TrimSpace(row[cbtIdx])) == "true"
		}

		diskCount := 0
		if idx, exists := colMap["disks"]; exists && idx < len(row) {
			diskCountStr := strings.Map(func(r rune) rune {
				if r >= '0' && r <= '9' {
					return r
				}
				return -1
			}, row[idx])

			if parsedCount, err := strconv.Atoi(diskCountStr); err == nil && parsedCount > 0 {
				diskCount = parsedCount
			}
		}

		totalDiskCapacityGB := 0
		if idx, exists := colMap["total disk capacity MiB"]; exists && idx < len(row) {
			capacityStr := strings.Map(func(r rune) rune {
				if (r >= '0' && r <= '9') || r == '.' {
					return r
				}
				return -1
			}, row[idx])

			if capacity, err := strconv.ParseFloat(capacityStr, 64); err == nil && capacity > 0 {
				totalDiskCapacityGB = int(capacity / 1024)
			}
		}

		if diskCount > 0 {
			capacityPerDisk := 10
			if totalDiskCapacityGB > 0 {
				capacityPerDisk = max(1, totalDiskCapacityGB/diskCount)
			}

			for j := 0; j < diskCount; j++ {
				vm.Disks = append(vm.Disks, Disk{
					Name:     "Disk_" + strconv.Itoa(j+1),
					Capacity: capacityPerDisk,
				})
			}
		} else if totalDiskCapacityGB > 0 {
			vm.Disks = append(vm.Disks, Disk{
				Name:     "Disk_1",
				Capacity: totalDiskCapacityGB,
			})
		}

		vms = append(vms, vm)
	}

	return vms, nil
}

func fillInventoryWithVMData(vms []VM, inventory *api.Inventory) {
	cpuSet := []int{}
	memorySet := []int{}
	diskGBSet := []int{}
	diskCountSet := []int{}

	if inventory.Vms.Os == nil {
		inventory.Vms.Os = make(map[string]int)
	}
	if inventory.Vms.PowerStates == nil {
		inventory.Vms.PowerStates = make(map[string]int)
	}

	vmWithCBTDisabled := 0

	inventory.Vms.Total = len(vms)
	inventory.Vms.TotalMigratable = len(vms)
	totalMigratableWithWarnings := 0

	for _, vm := range vms {
		cpuSet = append(cpuSet, vm.CPUCount)
		memorySet = append(memorySet, vm.MemoryMB/1024)
		
		totalDiskGB := 0
		for _, disk := range vm.Disks {
			totalDiskGB += disk.Capacity
		}
		
		diskGBSet = append(diskGBSet, totalDiskGB)
		diskCountSet = append(diskCountSet, len(vm.Disks))

		inventory.Vms.Os[vm.OS]++
		inventory.Vms.PowerStates[vm.PowerState]++

		inventory.Vms.CpuCores.Total += vm.CPUCount
		inventory.Vms.RamGB.Total += vm.MemoryMB / 1024
		inventory.Vms.DiskCount.Total += len(vm.Disks)
		inventory.Vms.DiskGB.Total += totalDiskGB

		if !vm.CBTEnabled {
			inventory.Vms.CpuCores.TotalForMigratableWithWarnings += vm.CPUCount
			inventory.Vms.RamGB.TotalForMigratableWithWarnings += vm.MemoryMB / 1024
			inventory.Vms.DiskCount.TotalForMigratableWithWarnings += len(vm.Disks)
			inventory.Vms.DiskGB.TotalForMigratableWithWarnings += totalDiskGB
			vmWithCBTDisabled++
			totalMigratableWithWarnings++
		} else {
			inventory.Vms.CpuCores.TotalForMigratable += vm.CPUCount
			inventory.Vms.RamGB.TotalForMigratable += vm.MemoryMB / 1024
			inventory.Vms.DiskCount.TotalForMigratable += len(vm.Disks)
			inventory.Vms.DiskGB.TotalForMigratable += totalDiskGB
		}
	}

	inventory.Vms.TotalMigratableWithWarnings = &totalMigratableWithWarnings

	inventory.Vms.CpuCores.Histogram = service.Histogram(cpuSet)
	inventory.Vms.RamGB.Histogram = service.Histogram(memorySet)
	inventory.Vms.DiskCount.Histogram = service.Histogram(diskCountSet)
	inventory.Vms.DiskGB.Histogram = service.Histogram(diskGBSet)

	if vmWithCBTDisabled > 0 {
		inventory.Vms.MigrationWarnings = nil
		inventory.Vms.MigrationWarnings = append(inventory.Vms.MigrationWarnings, struct {
			Assessment string `json:"assessment"`
			Count      int    `json:"count"`
			Label      string `json:"label"`
		}{
			Assessment: "Changed Block Tracking (CBT) has not been enabled on this VM. This feature is a prerequisite for VM warm migration.",
			Count:      vmWithCBTDisabled,
			Label:      "Changed Block Tracking (CBT) not enabled",
		})
	}
}