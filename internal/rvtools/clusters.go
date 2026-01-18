package rvtools

import (
	"crypto/sha256"
	"fmt"
	"sort"

	"github.com/kubev2v/migration-planner/internal/agent/service"
	"go.uber.org/zap"
)

// ExtractClusterIDMapping extracts cluster IDs and creates mappings from RVTools data
func ExtractClusterIDMapping(vInfoRows, vHostRows, vClusterRows [][]string, vcenterUUID string) service.ClusterIDMapping {
	clusterNameToID := buildClusterNameToIDMap(vClusterRows)

	hostToClusterID := buildHostToClusterIDMap(vHostRows, clusterNameToID, vcenterUUID)

	// Try to build VM to cluster mapping using host-based method first
	vmToClusterID := buildVMToClusterIDMap(vInfoRows, vHostRows, hostToClusterID)

	// If vHost sheet is missing/empty or no clusters found via host mapping,
	// fall back to reading cluster directly from vInfo column
	vHostSheetMissing := len(vHostRows) <= 1
	if vHostSheetMissing || len(vmToClusterID) == 0 {
		reason := "No clusters found via host mapping"
		if vHostSheetMissing {
			reason = "vHost sheet missing or empty"
		}
		zap.S().Named("rvtools").Infof("%s, trying direct cluster column from vInfo", reason)
		vmToClusterID = buildVMToClusterIDMapFromVInfo(vInfoRows, clusterNameToID, vcenterUUID)
	}

	clusterIDs := extractUniqueClusterIDs(vmToClusterID)

	return service.ClusterIDMapping{
		VMToClusterID:   vmToClusterID,
		HostToClusterID: hostToClusterID,
		ClusterIDs:      clusterIDs,
	}
}

func buildClusterNameToIDMap(vClusterRows [][]string) map[string]string {
	clusterNameToID := make(map[string]string)

	if len(vClusterRows) <= 1 {
		zap.S().Named("rvtools").Debugf("vCluster sheet not found or empty")
		return clusterNameToID
	}

	colMap := buildColumnMap(vClusterRows[0])

	for _, row := range vClusterRows[1:] {
		if len(row) == 0 {
			continue
		}

		clusterName := getColumnValue(row, colMap, "name")
		objectID := getColumnValue(row, colMap, "object id")

		if clusterName != "" && objectID != "" {
			clusterNameToID[clusterName] = objectID
			zap.S().Named("rvtools").Debugf("Mapped cluster '%s' -> '%s'", clusterName, objectID)
		}
	}

	zap.S().Named("rvtools").Infof("Found %d clusters in vCluster sheet", len(clusterNameToID))
	return clusterNameToID
}

func buildHostToClusterIDMap(vHostRows [][]string, clusterNameToID map[string]string, vcenterUUID string) map[string]string {
	if len(vHostRows) <= 1 {
		return make(map[string]string)
	}

	colMap := buildColumnMap(vHostRows[0])
	hostToCluster := make(map[string]string)

	for _, row := range vHostRows[1:] {
		if len(row) == 0 {
			continue
		}

		hostID := getColumnValue(row, colMap, "object id")
		clusterName := getColumnValue(row, colMap, "cluster")

		if hostID == "" || clusterName == "" {
			continue
		}

		// Look up cluster object ID from vCluster sheet mapping
		if clusterID, exists := clusterNameToID[clusterName]; exists {
			hostToCluster[hostID] = clusterID
		} else {
			// Fallback: generate from cluster name if not found in vCluster sheet
			datacenter := getColumnValue(row, colMap, "datacenter")
			clusterID := generateClusterID(clusterName, datacenter, vcenterUUID)
			hostToCluster[hostID] = clusterID
			zap.S().Named("rvtools").Warnf("Cluster '%s' not found in vCluster sheet, generated ID: %s", clusterName, clusterID)
		}
	}

	return hostToCluster
}

func buildVMToClusterIDMap(vInfoRows, vHostRows [][]string, hostToClusterID map[string]string) map[string]string {
	if len(vInfoRows) <= 1 {
		return make(map[string]string)
	}

	// Build Host IP/Name -> Host Object ID mapping from vHost
	hostIPToObjectID := createHostIPToObjectIDMap(vHostRows)

	vInfoColMap := buildColumnMap(vInfoRows[0])
	vmToCluster := make(map[string]string)

	for _, row := range vInfoRows[1:] {
		if len(row) == 0 {
			continue
		}

		vmName := getColumnValue(row, vInfoColMap, "vm")
		hostIP := getColumnValue(row, vInfoColMap, "host")

		if vmName == "" || hostIP == "" {
			continue
		}

		// Convert host IP to object ID, then to cluster ID
		if hostObjectID, exists := hostIPToObjectID[hostIP]; exists {
			if clusterID, exists := hostToClusterID[hostObjectID]; exists {
				vmToCluster[vmName] = clusterID
			}
		}
	}

	return vmToCluster
}

// buildVMToClusterIDMapFromVInfo builds VM to cluster mapping by reading cluster column
// directly from vInfo sheet when vHost sheet is missing or unavailable
func buildVMToClusterIDMapFromVInfo(vInfoRows [][]string, clusterNameToID map[string]string, vcenterUUID string) map[string]string {
	if len(vInfoRows) <= 1 {
		return make(map[string]string)
	}

	vInfoColMap := buildColumnMap(vInfoRows[0])
	vmToCluster := make(map[string]string)

	// Check if "cluster" column exists in vInfo
	if _, exists := vInfoColMap["cluster"]; !exists {
		zap.S().Named("rvtools").Debugf("Cluster column not found in vInfo sheet")
		return vmToCluster
	}

	for _, row := range vInfoRows[1:] {
		if len(row) == 0 {
			continue
		}

		vmName := getColumnValue(row, vInfoColMap, "vm")
		clusterName := getColumnValue(row, vInfoColMap, "cluster")

		if vmName == "" || clusterName == "" {
			continue
		}

		// Try to get cluster ID from vCluster sheet mapping first
		var clusterID string
		if id, exists := clusterNameToID[clusterName]; exists {
			clusterID = id
		} else {
			// Fallback: generate cluster ID from cluster name
			// Try to get datacenter from vInfo if available
			datacenter := getColumnValue(row, vInfoColMap, "datacenter")
			clusterID = generateClusterID(clusterName, datacenter, vcenterUUID)
			zap.S().Named("rvtools").Debugf("Generated cluster ID for '%s': %s", clusterName, clusterID)
		}

		vmToCluster[vmName] = clusterID
	}

	zap.S().Named("rvtools").Infof("Mapped %d VMs to clusters using vInfo cluster column", len(vmToCluster))
	return vmToCluster
}

func extractUniqueClusterIDs(vmToClusterID map[string]string) []string {
	clusterSet := make(map[string]struct{})

	for _, clusterID := range vmToClusterID {
		if clusterID != "" {
			clusterSet[clusterID] = struct{}{}
		}
	}

	clusterIDs := make([]string, 0, len(clusterSet))
	for clusterID := range clusterSet {
		clusterIDs = append(clusterIDs, clusterID)
	}
	sort.Strings(clusterIDs)

	if len(clusterIDs) > 0 {
		zap.S().Named("rvtools").Debugf("Extracted cluster IDs: %v", clusterIDs)
	}

	return clusterIDs
}

// generateClusterID creates a consistent anonymized cluster ID
// This ensures no cluster names are exposed in the output
func generateClusterID(clusterName, datacenterName, vcenterUUID string) string {
	// Combine all identifying info for uniqueness
	// Include vcenterUUID to avoid collisions across vCenters
	combined := fmt.Sprintf("%s:%s:%s", vcenterUUID, datacenterName, clusterName)
	hash := sha256.Sum256([]byte(combined))

	// Return as cluster-{first16hexchars}
	return fmt.Sprintf("cluster-%x", hash[:8])
}
