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
	
	networksMap := make(map[string]*struct {
		Name     string
		Type     string
		VlanId   string
		DVSwitch string
	})
	
	if len(dvswitchRows) > 1 {
		processDvSwitchSheet(dvswitchRows, dvswitchMap)
	}
	
	if len(dvportRows) > 1 {
		processDvPortSheet(dvportRows, dvswitchMap, networksMap)
	}

	if len(dvswitchMap) == 0 && len(networksMap) == 0 {
		return nil
	}
	
	inventory.Infra.Networks = []struct {
		Dvswitch *string               `json:"dvswitch,omitempty"`
		Name     string                `json:"name"`
		Type     api.InfraNetworksType `json:"type"`
		VlanId   *string               `json:"vlanId,omitempty"`
	}{}
	
	for switchName := range dvswitchMap {
		inventory.Infra.Networks = append(inventory.Infra.Networks, struct {
			Dvswitch *string               `json:"dvswitch,omitempty"`
			Name     string                `json:"name"`
			Type     api.InfraNetworksType `json:"type"`
			VlanId   *string               `json:"vlanId,omitempty"`
		}{
			Name: switchName,
			Type: api.InfraNetworksType(NetDvSwitch),
		})
	}
	
	for _, network := range networksMap {
		netEntry := struct {
			Dvswitch *string               `json:"dvswitch,omitempty"`
			Name     string                `json:"name"`
			Type     api.InfraNetworksType `json:"type"`
			VlanId   *string               `json:"vlanId,omitempty"`
		}{
			Name: network.Name,
			Type: api.InfraNetworksType(network.Type),
		}
		
		if network.DVSwitch != "" {
			dvSwitch := network.DVSwitch
			netEntry.Dvswitch = &dvSwitch
		}
		
		vlanId := network.VlanId
		netEntry.VlanId = &vlanId
		
		inventory.Infra.Networks = append(inventory.Infra.Networks, netEntry)
	}
	
	return nil
}

// processDvSwitchSheet extracts switch information from dvSwitch sheet
func processDvSwitchSheet(rows [][]string, dvswitchMap map[string]bool) {
	colMap := make(map[string]int)
	for i, header := range rows[0] {
		headerTrimmed := strings.TrimSpace(header)
		colMap[header] = i
		colMap[headerTrimmed] = i
		colMap[strings.ToLower(header)] = i
		colMap[strings.ToLower(headerTrimmed)] = i
	}

	switchIdx := -1
	for _, colName := range []string{"Switch", "Name"} {
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

func processDvPortSheet(rows [][]string, dvswitchMap map[string]bool, networksMap map[string]*struct {
    Name     string
    Type     string
    VlanId   string
    DVSwitch string
	}) {
	colMap := make(map[string]int)
	for i, header := range rows[0] {
		headerTrimmed := strings.TrimSpace(header)
		colMap[header] = i
		colMap[headerTrimmed] = i
		colMap[strings.ToLower(header)] = i
		colMap[strings.ToLower(headerTrimmed)] = i
	}

	portIdx := -1
	if idx, exists := colMap["Port"]; exists {
		portIdx = idx
	}
	
	switchIdx := -1
	if idx, exists := colMap["Switch"]; exists {
		switchIdx = idx
	}
	
	vlanIdx := -1
	if idx, exists := colMap["VLAN"]; exists {
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

		networksMap[portName] = &struct {
			Name     string
			Type     string
			VlanId   string
			DVSwitch string
		}{
			Name:     portName,
			Type:     NetDvPortGroup,
			VlanId:   vlanId,
			DVSwitch: switchName,
		}
	}
}