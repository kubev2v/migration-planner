package util

import (
	"encoding/json"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

// UnmarshalInventory decodes both legacy v1 inventories and v2 inventories.
// Optional fields remain unset so consumers can validate missing data.
func UnmarshalInventory(raw []byte, version uint) (api.Inventory, error) {
	if len(raw) == 0 {
		return api.Inventory{Clusters: make(map[string]api.InventoryData)}, nil
	}
	if version == model.InventoryVersionV1 {
		var data api.InventoryData
		if err := json.Unmarshal(raw, &data); err != nil {
			return api.Inventory{}, err
		}
		inventory := api.Inventory{Vcenter: &data, Clusters: make(map[string]api.InventoryData)}
		if data.Vcenter != nil {
			inventory.VcenterId = data.Vcenter.Id
		}
		return inventory, nil
	}
	var inventory api.Inventory
	if err := json.Unmarshal(raw, &inventory); err != nil {
		return api.Inventory{}, err
	}
	if inventory.Clusters == nil {
		inventory.Clusters = make(map[string]api.InventoryData)
	}
	return inventory, nil
}
