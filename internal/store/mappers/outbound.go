package mappers

import (
	"encoding/json"
	"fmt"

	"github.com/kubev2v/migration-planner/internal/util"

	"github.com/kubev2v/migration-planner/internal/model"
	storemodel "github.com/kubev2v/migration-planner/internal/store/model"
)

// AssessmentFromStore converts a persistence assessment (with raw jsonb
// inventories) to the service-layer domain model.
func AssessmentFromStore(a *storemodel.Assessment) (model.Assessment, error) {
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
	mainFound := false
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
		if !mainFound {
			main = inv
			mainFound = true
		}
	}
	return main, subsets, nil
}

// AssessmentInventoryFromStore converts a persistence inventory snapshot to the
// domain model, decoding the raw jsonb blob into a structured Inventory.
func AssessmentInventoryFromStore(si storemodel.AssessmentInventory) (model.AssessmentInventory, error) {
	inv, err := UnmarshalInventory(si.Inventory, si.Version)
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

// AssessmentSummaryListFromStore converts a slice of persistence assessments to
// lightweight domain summaries.
func AssessmentSummaryListFromStore(list []storemodel.Assessment) []model.AssessmentSummary {
	out := make([]model.AssessmentSummary, len(list))
	for i := range list {
		out[i] = AssessmentSummaryFromStore(list[i])
	}
	return out
}

// AssessmentSummaryFromStore converts a persistence assessment to a lightweight
// domain summary.
func AssessmentSummaryFromStore(a storemodel.Assessment) model.AssessmentSummary {
	s := model.AssessmentSummary{
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

	if len(a.Inventories) == 0 {
		return s
	}

	inv := a.Inventories[0]
	s.VMsCount = inv.VMsCount
	s.HostsCount = inv.HostsCount
	s.NetworksCount = inv.NetworksCount
	s.DatastoresCount = inv.DatastoresCount

	return s
}

// UnmarshalInventory decodes both legacy v1 inventories (a bare InventoryData)
// and v2 inventories into the structured domain Inventory. It mirrors
// internal/util.UnmarshalInventory but targets the domain types.
func UnmarshalInventory(raw []byte, version uint) (model.Inventory, error) {
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

// SourceFromStore decodes the optional inventory at the persistence boundary.
func SourceFromStore(s storemodel.Source) (model.Source, error) {
	result := model.Source{
		ID:          s.ID,
		Name:        s.Name,
		VCenterID:   s.VCenterID,
		Username:    s.Username,
		OrgID:       s.OrgID,
		OnPremises:  s.OnPremises,
		UpdateType:  s.UpdateType,
		EmailDomain: s.EmailDomain,
		CreatedAt:   s.CreatedAt, UpdatedAt: s.UpdatedAt,
		ImageInfra: ImageInfraFromStore(s.ImageInfra),
	}
	if len(s.Inventory) > 0 {
		result.Version = uint(util.GetInventoryVersion(s.Inventory))
		inventory, err := UnmarshalInventory(s.Inventory, result.Version)
		if err != nil {
			return model.Source{}, fmt.Errorf("failed to decode source %s inventory: %w", s.ID, err)
		}
		result.Inventory = &inventory
	}
	if s.Agents != nil {
		result.Agents = make([]model.Agent, len(s.Agents))
		for i, agent := range s.Agents {
			result.Agents[i] = AgentFromStore(agent)
		}
	}
	if s.Labels != nil {
		result.Labels = make([]model.Label, len(s.Labels))
		for i, label := range s.Labels {
			result.Labels[i] = LabelFromStore(label)
		}
	}
	return result, nil
}

// SourceSubsetInventoryFromStore decodes a stored subset inventory.
func SourceSubsetInventoryFromStore(s storemodel.SourceSubsetInventory) (model.SourceSubsetInventory, error) {
	version := uint(util.GetInventoryVersion(s.Inventory))
	inventory, err := UnmarshalInventory(s.Inventory, version)
	if err != nil {
		return model.SourceSubsetInventory{}, fmt.Errorf("failed to decode subset %q inventory: %w", s.Name, err)
	}
	return model.SourceSubsetInventory{
		ID:         s.ID,
		Name:       s.Name,
		SourceID:   s.SourceID,
		VCenterID:  s.VCenterID,
		VMsCount:   s.VMsCount,
		UpdateType: s.UpdateType,
		CreatedAt:  s.CreatedAt, UpdatedAt: s.UpdatedAt,
		Version: version, Inventory: inventory,
	}, nil
}

// AgentFromStore converts stored source metadata to domain values.
func AgentFromStore(s storemodel.Agent) model.Agent {
	return model.Agent{
		ID:         s.ID,
		Status:     s.Status,
		StatusInfo: s.StatusInfo,
		CredUrl:    s.CredUrl,
		Version:    s.Version,
		SourceID:   s.SourceID,
		CreatedAt:  s.CreatedAt,
		UpdatedAt:  s.UpdatedAt,
	}
}

// ImageInfraFromStore converts stored source metadata to domain values.
func ImageInfraFromStore(s storemodel.ImageInfra) model.ImageInfra {
	return model.ImageInfra{
		SourceID:         s.SourceID,
		HttpProxyUrl:     s.HttpProxyUrl,
		HttpsProxyUrl:    s.HttpsProxyUrl,
		NoProxyDomains:   s.NoProxyDomains,
		CertificateChain: s.CertificateChain,
		SshPublicKey:     s.SshPublicKey,
		ImageTokenKey:    s.ImageTokenKey,
		IpAddress:        s.IpAddress,
		SubnetMask:       s.SubnetMask,
		DefaultGateway:   s.DefaultGateway,
		Dns:              s.Dns,
		AgentVersion:     s.AgentVersion,
		AgentToken:       s.AgentToken,
		CreatedAt:        s.CreatedAt, UpdatedAt: s.UpdatedAt,
		ID: s.ID,
	}
}

// LabelFromStore converts stored source metadata to domain values.
func LabelFromStore(s storemodel.Label) model.Label {
	return model.Label{
		Key:      s.Key,
		Value:    s.Value,
		SourceID: s.SourceID,
	}
}
