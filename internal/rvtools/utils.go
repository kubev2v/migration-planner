package rvtools

import (
	"bytes"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
	"go.uber.org/zap"
)

var numberRegex = regexp.MustCompile(`[0-9.]+`)

func parseMemoryMB(s string) int32 {
	if s == "" {
		return 0
	}
	cleanS := strings.ReplaceAll(s, ",", "")	
	match := numberRegex.FindString(cleanS)
	if match == "" {
		return 0
	}
	if val, err := strconv.ParseFloat(match, 64); err == nil {
		return int32(val)
	}
	return 0
}

func parseIntOrZero(s string) int32 {
	s = strings.ReplaceAll(s, ",", "")
	val, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return int32(val)
}

func parseFormattedInt64(s string) int64 {
	cleanStr := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, strings.TrimSpace(s))

	if cleanStr == "" {
		return 0
	}

	val, err := strconv.ParseInt(cleanStr, 10, 64)
	if err != nil {
		zap.S().Warnf("Invalid numeric string: %s", cleanStr)
		return 0
	}
	return val
}


func parseBooleanValue(s string) bool {
	if s == "" {
		return false
	}
	cleanStr := strings.ToLower(strings.TrimSpace(s))
	return cleanStr == "true" || cleanStr == "1" || cleanStr == "yes" || cleanStr == "enabled"
}

func getColumnValue(row []string, colMap map[string]int, key string) string {
	if idx, exists := colMap[key]; exists && idx < len(row) {
		return strings.TrimSpace(row[idx])
	}
	return ""
}

func buildColumnMap(headers []string) map[string]int {
	colMap := make(map[string]int)
	for i, header := range headers {
		key := strings.ToLower(strings.TrimSpace(header))
		colMap[key] = i
	}
	return colMap
}

func ensureMapExists[K comparable, V any](m map[K]map[K]V, key K) {
	if _, ok := m[key]; !ok {
		m[key] = make(map[K]V)
	}
}

func calculateHostsPerCluster(clusterToHosts map[string]map[string]struct{}) []int {
	if len(clusterToHosts) == 0 {
		return []int{}
	}
	
	// Sort cluster names for consistent ordering
	clusterNames := make([]string, 0, len(clusterToHosts))
	for cluster := range clusterToHosts {
		clusterNames = append(clusterNames, cluster)
	}
	sort.Strings(clusterNames)
	
	hostsPerCluster := make([]int, 0, len(clusterNames))
	for _, cluster := range clusterNames {
		hostsPerCluster = append(hostsPerCluster, len(clusterToHosts[cluster]))
	}
	
	return hostsPerCluster
}

func calculateClustersPerDatacenter(datacenterToClusters map[string]map[string]struct{}) []int {
	if len(datacenterToClusters) == 0 {
		return []int{}
	}
	
	clustersPerDatacenter := make([]int, 0, len(datacenterToClusters))
	for _, clusters := range datacenterToClusters {
		clustersPerDatacenter = append(clustersPerDatacenter, len(clusters))
	}
	
	return clustersPerDatacenter
}

func groupRowsByVM(rows [][]string, colMap map[string]int) map[string][][]string {
	vmData := make(map[string][][]string)
	
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		
		vmName := getColumnValue(row, colMap, "vm")
		if vmName == "" {
			continue
		}
		
		vmData[vmName] = append(vmData[vmName], row)
	}
	
	return vmData
}

func parseDatastoreFromPath(path string) string {
	// Path format: [datastoreName] path/to/file.vmdk
	if strings.HasPrefix(path, "[") {
		endIdx := strings.Index(path, "]")
		if endIdx > 1 {
			return path[1:endIdx]
		}
	}
	return ""
}

func buildDatastoreMapping(datastoreRows [][]string) map[string]string {
	datastoreNameToID := make(map[string]string)
	
	if len(datastoreRows) <= 1 {
		return datastoreNameToID
	}
	
	headers := datastoreRows[0]
	colMap := make(map[string]int)
	for i, header := range headers {
		key := strings.ToLower(strings.TrimSpace(header))
		colMap[key] = i
	}
	
	for _, row := range datastoreRows[1:] {
		if len(row) == 0 {
			continue
		}
		
		// Extract datastore name and object ID
		datastoreName := getColumnValue(row, colMap, "name")
		objectID := getColumnValue(row, colMap, "object id")
		
		if datastoreName != "" && objectID != "" {
			datastoreNameToID[datastoreName] = objectID
		}
	}
	
	return datastoreNameToID
}

func readSheet(excelFile *excelize.File, sheets []string, sheetName string) [][]string {
	if !slices.Contains(sheets, sheetName) {
		return [][]string{}
	}

	rows, err := excelFile.GetRows(sheetName)
	if err != nil {
		zap.S().Named("rvtools").Warnf("Could not read %s sheet: %v", sheetName, err)
		return [][]string{}
	}

	return rows
}

func createNetworkNameToIDMap(dvPortRows [][]string) map[string]string {
	networkMap := make(map[string]string)
	
	if len(dvPortRows) <= 1 {
		return networkMap // Return empty map if no dvPort data
	}

	dvPortColMap := buildColumnMap(dvPortRows[0])

	for _, row := range dvPortRows[1:] {
		if len(row) == 0 {
			continue
		}

		networkName := getColumnValue(row, dvPortColMap, "port")
		objectID := getColumnValue(row, dvPortColMap, "object id")
		
		if networkName != "" && objectID != "" {
			networkMap[networkName] = objectID
		}
	}
	
	return networkMap
}

func IsExcelFile(content []byte) bool {
	if len(content) < 2 {
		return false
	}

	if content[0] == 0x50 && content[1] == 0x4B {
		f, err := excelize.OpenReader(bytes.NewReader(content))
		if err != nil {
			return false
		}
		defer f.Close()
		return true
	}

	return false
}

func splitSheet(rows [][]string) (header []string, data [][]string) {
	if len(rows) == 0 {
		return []string{}, [][]string{}
	}
	return rows[0], rows[1:]
}

func (ct *ControllerTracker) GetControllerKey(controllerName string) int32 {
	controllerName = strings.ToLower(controllerName)
	
	switch {
	case strings.Contains(controllerName, "ide"):
		key := 200 + ct.ideCount
		ct.ideCount++
		return key
	case strings.Contains(controllerName, "scsi"):
		key := 1000 + ct.scsiCount
		ct.scsiCount++
		return key
	case strings.Contains(controllerName, "sata"):
		key := 15000 + ct.sataCount
		ct.sataCount++
		return key
	case strings.Contains(controllerName, "nvme"):
		key := 20000 + ct.nvmeCount
		ct.nvmeCount++
		return key
	default:
		key := 1000 + ct.scsiCount
		ct.scsiCount++
		return key
	}
}

func mapControllerNameToBus(controllerName string) string {
	controllerLower := strings.ToLower(controllerName)
	if strings.Contains(controllerLower, "scsi") {
		return "scsi"
	} else if strings.Contains(controllerLower, "ide") {
		return "ide"
	} else if strings.Contains(controllerLower, "sata") {
		return "sata"
	} else if strings.Contains(controllerLower, "nvme") {
		return "nvme"
	}
	return "scsi" // default
}


func cleanNumericString(s string) string {
	return strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || r == '.' {
			return r
		}
		return -1
	}, s)
}