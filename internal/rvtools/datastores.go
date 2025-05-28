package rvtools

import (
	"strconv"
	"strings"
	
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
)

func processDatastoreInfo(rows [][]string, inventory *api.Inventory) error {
    if len(rows) <= 1 {
        return nil // or return fmt.Errorf("no valid datastore data available")
    }

    headers := rows[0]

    colMap := make(map[string]int)
    for i, header := range headers {
        headerTrimmed := strings.TrimSpace(header)
        colMap[header] = i
        colMap[headerTrimmed] = i
        colMap[strings.ToLower(header)] = i
        colMap[strings.ToLower(headerTrimmed)] = i
    }

    nameIdx := colMap["Name"]
    objectIdIdx := colMap["Object ID"]
    typeIdx := colMap["Type"] 
    capacityMiBIdx := colMap["Capacity MiB"]
    freeMiBIdx := colMap["Free MiB"]
    freePercentIdx := colMap["Free %"]

    for i := 1; i < len(rows); i++ {
        row := rows[i]
        if len(row) == 0 {
            continue
        }

        if len(row) <= nameIdx || len(row) <= capacityMiBIdx {
            continue
        }
        
        datastore := api.Datastore{
            DiskId:                  "",
            HardwareAcceleratedMove: true,
            TotalCapacityGB:         0,
            Type:                    "VMFS",
            Vendor:                  "ATA",
        }
        
        if nameIdx >= 0 && nameIdx < len(row) {
            datastore.DiskId = row[nameIdx]
        }
        
        if datastore.DiskId == "" {
            continue
        }
        
        if objectIdIdx >= 0 && objectIdIdx < len(row) && row[objectIdIdx] != "" {
            datastore.DiskId = row[objectIdIdx]
        }
        
        if typeIdx >= 0 && typeIdx < len(row) && row[typeIdx] != "" {
            datastore.Type = row[typeIdx]
        }
        
        if capacityMiBIdx >= 0 && capacityMiBIdx < len(row) {
            capacityStr := row[capacityMiBIdx]
            capacityStr = strings.Map(func(r rune) rune {
                if (r >= '0' && r <= '9') || r == '.' {
                    return r
                }
                return -1
            }, capacityStr)
            
            capacityMiB, err := strconv.ParseFloat(capacityStr, 64)
            if err == nil && capacityMiB > 0 {
                datastore.TotalCapacityGB = int(capacityMiB / 1024)
                if datastore.TotalCapacityGB == 0 {
                    datastore.TotalCapacityGB = 1
                }
            }
        }
        
        if freeMiBIdx >= 0 && freeMiBIdx < len(row) {
            freeStr := row[freeMiBIdx]
            freeStr = strings.Map(func(r rune) rune {
                if (r >= '0' && r <= '9') || r == '.' {
                    return r
                }
                return -1
            }, freeStr)
            
            freeMiB, err := strconv.ParseFloat(freeStr, 64)
            if err == nil && freeMiB >= 0 {
                datastore.FreeCapacityGB = int(freeMiB / 1024)
            }
        } else if freePercentIdx >= 0 && freePercentIdx < len(row) && datastore.TotalCapacityGB > 0 {
            freePercentStr := row[freePercentIdx]
            freePercentStr = strings.Map(func(r rune) rune {
                if (r >= '0' && r <= '9') || r == '.' {
                    return r
                }
                return -1
            }, freePercentStr)
            
            freePercent, err := strconv.ParseFloat(freePercentStr, 64)
            if err == nil && freePercent >= 0 {
                if freePercent > 1.0 {
                    freePercent = freePercent / 100.0
                }
                
                datastore.FreeCapacityGB = int(float64(datastore.TotalCapacityGB) * freePercent)
            }
        }

        if datastore.FreeCapacityGB > datastore.TotalCapacityGB {
            datastore.FreeCapacityGB = datastore.TotalCapacityGB
        }

        if datastore.TotalCapacityGB == 0 {
            datastore.TotalCapacityGB = 500
        }
        
        if datastore.FreeCapacityGB == 0 {
            datastore.FreeCapacityGB = datastore.TotalCapacityGB / 5
        }

        inventory.Infra.Datastores = append(inventory.Infra.Datastores, datastore)
    }
    
    return nil
}
