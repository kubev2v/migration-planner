package rvtools

import (
	"strconv"
	"strings"
	
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
)

func processDatastoreInfo(rows [][]string, inventory *api.Inventory) error {
    if len(rows) <= 1 {
        return nil
    }

    headers := rows[0]

    colMap := make(map[string]int)
    for i, header := range headers {
        key := strings.ToLower(strings.TrimSpace(header))
        colMap[key] = i
    }

    nameIdx := colMap["name"]
    objectIdIdx := colMap["object id"]
    typeIdx := colMap["Type"] 
    capacityMiBIdx, capacityMiBIdxOk := colMap["capacity mib"]
    freeMiBIdx := colMap["free mib"]
    freePercentIdx := colMap["free %"]

    for i := 1; i < len(rows); i++ {
        row := rows[i]
        if len(row) == 0 {
            continue
        }

        if len(row) <= nameIdx || len(row) <= capacityMiBIdx {
            continue
        }
        
        datastore := api.Datastore{
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
        
        if capacityMiBIdxOk {
            capacityStr := cleanNumericString(row[capacityMiBIdx])
            
            capacityMiB, err := strconv.ParseFloat(capacityStr, 64)
            if err == nil && capacityMiB > 0 {
                datastore.TotalCapacityGB = int(capacityMiB / 1024)
                if datastore.TotalCapacityGB == 0 {
                    datastore.TotalCapacityGB = 1
                }
            }
        }
        
        if freeMiBIdx >= 0 && freeMiBIdx < len(row) {
            freeStr := cleanNumericString(row[freeMiBIdx])
            
            freeMiB, err := strconv.ParseFloat(freeStr, 64)
            if err == nil && freeMiB >= 0 {
                datastore.FreeCapacityGB = int(freeMiB / 1024)
            }
        } else if freePercentIdx >= 0 && freePercentIdx < len(row) && datastore.TotalCapacityGB > 0 {
            freePercentStr := cleanNumericString(row[freePercentIdx])
            
            freePercent, err := strconv.ParseFloat(freePercentStr, 64)
            if err == nil && freePercent >= 0 {
                if freePercent > 1.0 {
                    freePercent = freePercent / 100.0
                }
                
                datastore.FreeCapacityGB = int(float64(datastore.TotalCapacityGB) * freePercent)
            }
        }
        
        // Ensure that the free capacity of the datastore does not exceed its total capacity.
        // If FreeCapacityGB is greater than TotalCapacityGB, set FreeCapacityGB to TotalCapacityGB.
        if datastore.FreeCapacityGB > datastore.TotalCapacityGB {
            datastore.FreeCapacityGB = datastore.TotalCapacityGB
        }

        inventory.Infra.Datastores = append(inventory.Infra.Datastores, datastore)
    }
    
    return nil
}

func cleanNumericString(s string) string {
    return strings.Map(func(r rune) rune {
        if (r >= '0' && r <= '9') || r == '.' {
            return r
        }
        return -1
    }, s)
}