package mappers

import (
	"encoding/json"
	"fmt"

	svcmodel "github.com/kubev2v/migration-planner/internal/service/model"
	storemodel "github.com/kubev2v/migration-planner/internal/store/model"
)

// AssessmentFromStore converts a persistence assessment (with raw jsonb
// inventories) to the service-layer domain model.
func AssessmentFromStore(a storemodel.Assessment) (svcmodel.Assessment, error) {
	main, subsets, err := assessmentInventoriesFromStore(a.Inventories)
	if err != nil {
		return svcmodel.Assessment{}, err
	}
	return svcmodel.Assessment{
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

// AssessmentSummaryListFromStore converts a slice of persistence assessments to
// lightweight domain summaries. When includeInventories is true, each summary is
// populated with the full structured inventories.
func AssessmentSummaryListFromStore(list []storemodel.Assessment, includeInventories bool) ([]svcmodel.AssessmentSummary, error) {
	out := make([]svcmodel.AssessmentSummary, len(list))
	for i := range list {
		s, err := AssessmentSummaryFromStore(list[i], includeInventories)
		if err != nil {
			return nil, err
		}
		out[i] = s
	}
	return out, nil
}

// AssessmentSummaryFromStore converts a persistence assessment to a lightweight
// domain summary. The resource counts are taken from the main (non-subset)
// inventory's stored counters without decoding the inventory blob. When
// includeInventories is true, the full structured inventories are also decoded
// and attached.
func AssessmentSummaryFromStore(a storemodel.Assessment, includeInventories bool) (svcmodel.AssessmentSummary, error) {
	s := svcmodel.AssessmentSummary{
		ID:             a.ID,
		CreatedAt:      a.CreatedAt,
		Name:           a.Name,
		OrgID:          a.OrgID,
		Username:       a.Username,
		OwnerFirstName: a.OwnerFirstName,
		OwnerLastName:  a.OwnerLastName,
		SourceType:     a.SourceType,
		SourceID:       a.SourceID,
		Permissions:    PermissionsFromStore(a.Permissions),
		Sharing:        SharingFromStore(a.Sharing),
	}

	for _, inv := range a.Inventories {
		if inv.IsSubset {
			continue
		}
		s.VMsCount = inv.VMsCount
		s.HostsCount = inv.HostsCount
		s.NetworksCount = inv.NetworksCount
		s.DatastoresCount = inv.DatastoresCount
		break
	}

	if includeInventories {
		main, subsets, err := assessmentInventoriesFromStore(a.Inventories)
		if err != nil {
			return svcmodel.AssessmentSummary{}, err
		}
		s.MainInventory = main
		s.SubsetInventories = subsets
	}

	return s, nil
}

func assessmentInventoriesFromStore(inventories []storemodel.AssessmentInventory) (svcmodel.AssessmentInventory, []svcmodel.AssessmentInventory, error) {
	var main svcmodel.AssessmentInventory
	subsets := make([]svcmodel.AssessmentInventory, 0)
	for _, stored := range inventories {
		inv, err := AssessmentInventoryFromStore(stored)
		if err != nil {
			return svcmodel.AssessmentInventory{}, nil, fmt.Errorf("failed to convert inventory %q: %w", stored.Name, err)
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
func AssessmentInventoryFromStore(si storemodel.AssessmentInventory) (svcmodel.AssessmentInventory, error) {
	inv, err := unmarshalInventory(si.Inventory, si.Version)
	if err != nil {
		return svcmodel.AssessmentInventory{}, err
	}
	return svcmodel.AssessmentInventory{
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
func unmarshalInventory(raw []byte, version uint) (svcmodel.Inventory, error) {
	if len(raw) == 0 {
		return svcmodel.Inventory{Clusters: make(map[string]svcmodel.InventoryData)}, nil
	}
	if version == svcmodel.InventoryVersionV1 {
		var data svcmodel.InventoryData
		if err := json.Unmarshal(raw, &data); err != nil {
			return svcmodel.Inventory{}, fmt.Errorf("failed to unmarshal v1 inventory: %w", err)
		}
		inv := svcmodel.Inventory{Vcenter: &data, Clusters: make(map[string]svcmodel.InventoryData)}
		if data.Vcenter != nil {
			inv.VcenterId = data.Vcenter.Id
		}
		return inv, nil
	}
	var inv svcmodel.Inventory
	if err := json.Unmarshal(raw, &inv); err != nil {
		return svcmodel.Inventory{}, fmt.Errorf("failed to unmarshal v2 inventory: %w", err)
	}
	if inv.Clusters == nil {
		inv.Clusters = make(map[string]svcmodel.InventoryData)
	}
	return inv, nil
}
