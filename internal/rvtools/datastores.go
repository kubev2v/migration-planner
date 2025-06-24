package rvtools

import (
	"strconv"
	"strings"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"go.uber.org/zap"
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

	for i := 1; i < len(rows); i++ {
		row := rows[i]
		if len(row) == 0 {
			continue
		}

		name := getColumnValue(row, colMap, "name")
		dsType := getColumnValue(row, colMap, "type")
		
		if name == "" {
			continue
		}

		datastore := api.Datastore{
			DiskId:                  name,
			Type:                    dsType,
			Vendor:                  "N/A",
			Model:                   "N/A",
			ProtocolType:            "N/A",
			HardwareAcceleratedMove: false,
		}

		capacityStr := getColumnValue(row, colMap, "capacity mib")
		if capacityStr != "" {
			if capacityMiB, err := strconv.ParseFloat(cleanNumericString(capacityStr), 64); err == nil && capacityMiB > 0 {
				datastore.TotalCapacityGB = int(capacityMiB / 1024)
			}
		}

		freeStr := getColumnValue(row, colMap, "free mib")
		if freeStr != "" {
			if freeMiB, err := strconv.ParseFloat(cleanNumericString(freeStr), 64); err == nil && freeMiB >= 0 {
				datastore.FreeCapacityGB = int(freeMiB / 1024)
			}
		}

		if datastore.FreeCapacityGB > datastore.TotalCapacityGB {
			datastore.FreeCapacityGB = datastore.TotalCapacityGB
		}

		inventory.Infra.Datastores = append(inventory.Infra.Datastores, datastore)
	}

	zap.S().Named("rvtools").Infof("Processed %d datastores", len(inventory.Infra.Datastores))
	return nil
}

func correlateDatastoreInfo(multipathRows, hbaRows [][]string, inventory *api.Inventory) {
	if len(inventory.Infra.Datastores) == 0 {
		return
	}

	multipathInfo := extractMultipathInfo(multipathRows)

	hasISCSIAdapter := hasISCSIHBA(hbaRows)

	for i := range inventory.Infra.Datastores {
		ds := &inventory.Infra.Datastores[i]
		
		if info, exists := multipathInfo[ds.DiskId]; exists {
			ds.DiskId = info.NAA
			
			if info.Vendor != "" {
				ds.Vendor = info.Vendor
			}
			if info.Model != "" {
				ds.Model = info.Model
			}

			if strings.HasPrefix(info.NAA, "naa.") {
				ds.ProtocolType = "iSCSI"
			} else if strings.HasPrefix(info.NAA, "mpx.") {
				ds.ProtocolType = "SAS"
			}
		} else {
			if strings.ToUpper(ds.Type) == "NFS" {
				ds.DiskId = "N/A"
				ds.ProtocolType = "N/A"
			} else if hasISCSIAdapter && strings.Contains(ds.DiskId, "naa.") {
				ds.ProtocolType = "iSCSI"
			}
		}
	}
}

func extractMultipathInfo(multipathRows [][]string) map[string]struct {
	NAA    string
	Vendor string
	Model  string
} {
	multipathInfo := make(map[string]struct {
		NAA    string
		Vendor string
		Model  string
	})

	if len(multipathRows) <= 1 {
		return multipathInfo
	}

	headers := multipathRows[0]
	colMap := make(map[string]int)
	for i, header := range headers {
		key := strings.ToLower(strings.TrimSpace(header))
		colMap[key] = i
	}

	for i := 1; i < len(multipathRows); i++ {
		row := multipathRows[i]
		if len(row) == 0 {
			continue
		}

		datastoreName := getColumnValue(row, colMap, "datastore")
		naaIdentifier := getColumnValue(row, colMap, "disk")
		vendor := getColumnValue(row, colMap, "vendor")
		model := getColumnValue(row, colMap, "model")

		if datastoreName != "" && naaIdentifier != "" {
			multipathInfo[datastoreName] = struct {
				NAA    string
				Vendor string
				Model  string
			}{
				NAA:    naaIdentifier,
				Vendor: vendor,
				Model:  model,
			}
		}
	}

	return multipathInfo
}

func hasISCSIHBA(hbaRows [][]string) bool {
	if len(hbaRows) <= 1 {
		return false
	}

	headers := hbaRows[0]
	colMap := make(map[string]int)
	for i, header := range headers {
		key := strings.ToLower(strings.TrimSpace(header))
		colMap[key] = i
	}

	for i := 1; i < len(hbaRows); i++ {
		row := hbaRows[i]
		if len(row) == 0 {
			continue
		}

		hbaType := getColumnValue(row, colMap, "type")
		if hbaType == "ISCSI" {
			return true
		}
	}

	return false
}
