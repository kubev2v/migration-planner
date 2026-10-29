package mappers

import (
	"encoding/json"
	"fmt"

	svcmodel "github.com/kubev2v/migration-planner/internal/model"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

// AssessmentToStore serializes a domain assessment at the persistence boundary.
func AssessmentToStore(a svcmodel.Assessment) (model.Assessment, error) {
	result := model.Assessment{
		ID: a.ID, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
		Name: a.Name, OrgID: a.OrgID, Username: a.Username,
		OwnerFirstName: a.OwnerFirstName, OwnerLastName: a.OwnerLastName,
		SourceType: a.SourceType, SourceID: a.SourceID,
	}
	combined := make([]svcmodel.AssessmentInventory, 0, 1+len(a.SubsetInventories))
	combined = append(combined, a.MainInventory)
	combined = append(combined, a.SubsetInventories...)

	storeInventories, err := AssessmentInventoriesToStore(combined...)
	if err != nil {
		return model.Assessment{}, fmt.Errorf("failed to convert inventories to store: %w", err)
	}

	result.Inventories = storeInventories

	return result, nil
}

func AssessmentInventoriesToStore(inventories ...svcmodel.AssessmentInventory) ([]model.AssessmentInventory, error) {
	storedInventories := make([]model.AssessmentInventory, 0, len(inventories))
	for _, inventory := range inventories {
		stored, err := assessmentInventoryToStore(inventory)
		if err != nil {
			return nil, fmt.Errorf("failed to convert inventory %q: %w", inventory.Name, err)
		}
		storedInventories = append(storedInventories, stored)
	}
	return storedInventories, nil
}

func assessmentInventoryToStore(inventory svcmodel.AssessmentInventory) (model.AssessmentInventory, error) {
	var data any = inventory.Inventory
	if inventory.Version == svcmodel.InventoryVersionV1 {
		data = inventory.Inventory.Vcenter
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return model.AssessmentInventory{}, fmt.Errorf("failed to marshal inventory: %w", err)
	}
	return model.AssessmentInventory{
		ID: inventory.ID, CreatedAt: inventory.CreatedAt, Name: inventory.Name,
		AssessmentID: inventory.AssessmentID, IsSubset: inventory.IsSubset,
		Version: inventory.Version, VCenterID: inventory.VCenterID, Inventory: raw,
		VMsCount: inventory.VMsCount, HostsCount: inventory.HostsCount,
		NetworksCount: inventory.NetworksCount, DatastoresCount: inventory.DatastoresCount,
	}, nil
}
