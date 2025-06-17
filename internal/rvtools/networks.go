// networks.go
package rvtools

import (
	"strings"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
)

const (
	NetStandard    = "standard"
	NetDvSwitch    = "dvswitch"
	NetDvPortGroup = "distributed"
)

func processNetworkInfo(dvswitchRows [][]string, dvportRows [][]string, inventory *api.Inventory) error {

	dvswitchMap := make(map[string]bool)

	networksMap := make(map[string]api.Network)

	if len(dvswitchRows) > 1 {
		processDvSwitchSheet(dvswitchRows, dvswitchMap)
	}

	if len(dvportRows) > 1 {
		processDvPortSheet(dvportRows, dvswitchMap, networksMap)
	}

	if len(dvswitchMap) == 0 && len(networksMap) == 0 {
		return nil
	}

	inventory.Infra.Networks = []api.Network{}

	for switchName := range dvswitchMap {
		inventory.Infra.Networks = append(inventory.Infra.Networks, api.Network{
			Name: switchName,
			Type: api.NetworkType(NetDvSwitch),
		})
	}

	for _, network := range networksMap {
		netEntry := api.Network{
			Name: network.Name,
			Type: api.NetworkType(network.Type),
		}

		if network.Dvswitch != nil && *network.Dvswitch != "" {
			netEntry.Dvswitch = network.Dvswitch
		}

		netEntry.VlanId = network.VlanId
		inventory.Infra.Networks = append(inventory.Infra.Networks, netEntry)
	}

	return nil
}

// processDvSwitchSheet extracts switch information from dvSwitch sheet
func processDvSwitchSheet(rows [][]string, dvswitchMap map[string]bool) {
	colMap := make(map[string]int)
	for i, header := range rows[0] {
		key := strings.ToLower(strings.TrimSpace(header))
		colMap[key] = i
	}

	switchIdx := -1
	for _, colName := range []string{"switch", "name"} {
		if idx, exists := colMap[colName]; exists {
			switchIdx = idx
			break
		}
	}

	if switchIdx == -1 {
		return
	}

	for i := 1; i < len(rows); i++ {
		row := rows[i]
		if len(row) <= switchIdx {
			continue
		}

		switchName := strings.TrimSpace(row[switchIdx])
		if switchName != "" {
			dvswitchMap[switchName] = true
		}
	}
}

func processDvPortSheet(rows [][]string, dvswitchMap map[string]bool, networksMap map[string]api.Network) {
	colMap := make(map[string]int)
	for i, header := range rows[0] {
		key := strings.ToLower(strings.TrimSpace(header))
		colMap[key] = i
	}

	portIdx := -1
	if idx, exists := colMap["port"]; exists {
		portIdx = idx
	}

	switchIdx := -1
	if idx, exists := colMap["switch"]; exists {
		switchIdx = idx
	}

	vlanIdx := -1
	if idx, exists := colMap["vlan"]; exists {
		vlanIdx = idx
	}

	if portIdx == -1 || switchIdx == -1 {
		return
	}

	for i := 1; i < len(rows); i++ {
		row := rows[i]
		if len(row) <= portIdx || len(row) <= switchIdx {
			continue
		}

		portName := strings.TrimSpace(row[portIdx])
		switchName := strings.TrimSpace(row[switchIdx])

		if portName == "" || switchName == "" {
			continue
		}

		dvswitchMap[switchName] = true

		if _, exists := networksMap[portName]; exists {
			continue
		}

		vlanId := ""
		if vlanIdx >= 0 && vlanIdx < len(row) {
			vlanId = strings.TrimSpace(row[vlanIdx])
		}

		networksMap[portName] = api.Network{
			Name:     portName,
			Type:     NetDvPortGroup,
			VlanId:   &vlanId,
			Dvswitch: &switchName,
		}
	}
}
