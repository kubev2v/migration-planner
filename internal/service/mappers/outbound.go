package mappers

import (
	"encoding/json"
	"fmt"

	"github.com/kubev2v/migration-planner/internal/model"

	storemodel "github.com/kubev2v/migration-planner/internal/store/model"
)

// AssessmentFromStore converts a persistence assessment (with raw jsonb
// inventories) to the service-layer domain model.
func AssessmentFromStore(a storemodel.Assessment) (model.Assessment, error) {
	main, subsets, err := assessmentInventoriesFromStore(a.Inventories)
	if err != nil {
		return model.Assessment{}, err
	}
	return model.Assessment{
		ID:                a.ID,
		CreatedAt:         a.CreatedAt,
		UpdatedAt:         a.UpdatedAt,
		Name:              a.Name,
		OrgID:             a.OrgID,
		Username:          a.Username,
		OwnerFirstName:    a.OwnerFirstName,
		OwnerLastName:     a.OwnerLastName,
		SourceType:        a.SourceType,
		SourceID:          a.SourceID,
		MainInventory:     main,
		SubsetInventories: subsets,
		Permissions:       PermissionsFromStore(a.Permissions),
		Sharing:           SharingFromStore(a.Sharing),
	}, nil
}

func assessmentInventoriesFromStore(inventories []storemodel.AssessmentInventory) (model.AssessmentInventory, []model.AssessmentInventory, error) {
	var main model.AssessmentInventory
	subsets := make([]model.AssessmentInventory, 0)
	for _, stored := range inventories {
		inv, err := AssessmentInventoryFromStore(stored)
		if err != nil {
			return model.AssessmentInventory{}, nil, fmt.Errorf("failed to convert inventory %q: %w", stored.Name, err)
		}
		if inv.IsSubset {
			subsets = append(subsets, inv)
			continue
		}
		main = inv
	}
	return main, subsets, nil
}

// AssessmentInventoryFromStore converts a persistence inventory snapshot to the
// domain model, decoding the raw jsonb blob into a structured Inventory.
func AssessmentInventoryFromStore(si storemodel.AssessmentInventory) (model.AssessmentInventory, error) {
	inv, err := unmarshalInventory(si.Inventory, si.Version)
	if err != nil {
		return model.AssessmentInventory{}, err
	}
	return model.AssessmentInventory{
		ID:              si.ID,
		CreatedAt:       si.CreatedAt,
		Name:            si.Name,
		IsSubset:        si.IsSubset,
		VCenterID:       si.VCenterID,
		VMsCount:        si.VMsCount,
		HostsCount:      si.HostsCount,
		NetworksCount:   si.NetworksCount,
		DatastoresCount: si.DatastoresCount,
		Version:         si.Version,
		Inventory:       inv,
		AssessmentID:    si.AssessmentID,
	}, nil
}

// unmarshalInventory decodes both legacy v1 inventories (a bare InventoryData)
// and v2 inventories into the structured domain Inventory. It mirrors
// internal/util.UnmarshalInventory but targets the domain types.
func unmarshalInventory(raw []byte, version uint) (model.Inventory, error) {
	if len(raw) == 0 {
		return model.Inventory{Clusters: make(map[string]model.InventoryData)}, nil
	}
	if version == model.InventoryVersionV1 {
		var data model.InventoryData
		if err := json.Unmarshal(raw, &data); err != nil {
			return model.Inventory{}, fmt.Errorf("failed to unmarshal v1 inventory: %w", err)
		}
		inv := model.Inventory{Vcenter: &data, Clusters: make(map[string]model.InventoryData)}
		if data.Vcenter != nil {
			inv.VcenterId = data.Vcenter.Id
		}
		return inv, nil
	}
	var inv model.Inventory
	if err := json.Unmarshal(raw, &inv); err != nil {
		return model.Inventory{}, fmt.Errorf("failed to unmarshal v2 inventory: %w", err)
	}
	if inv.Clusters == nil {
		inv.Clusters = make(map[string]model.InventoryData)
	}
	return inv, nil
}

// PermissionsFromStore converts stored permissions to independent service values.
func PermissionsFromStore(permissions []storemodel.Permission) []model.Permission {
	if permissions == nil {
		return nil
	}
	result := make([]model.Permission, len(permissions))
	for i, permission := range permissions {
		result[i] = model.Permission(permission)
	}
	return result
}

// SharingFromStore converts stored sharing metadata to independent service values.
func SharingFromStore(sharing *storemodel.Sharing) *model.Sharing {
	if sharing == nil {
		return nil
	}
	result := &model.Sharing{IsShared: sharing.IsShared}
	if sharing.SharedWith != nil {
		result.SharedWith = make([]model.SharingSubject, len(sharing.SharedWith))
		for i, subject := range sharing.SharedWith {
			result.SharedWith[i] = model.SharingSubject{
				Type: subject.Type,
				ID:   subject.ID,
				Name: subject.Name,
			}
		}
	}
	if sharing.SharedBy != nil {
		result.SharedBy = &model.SharingSubject{
			Type: sharing.SharedBy.Type,
			ID:   sharing.SharedBy.ID,
			Name: sharing.SharedBy.Name,
		}
	}
	return result
}
