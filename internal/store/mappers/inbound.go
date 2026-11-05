package mappers

import (
	"encoding/json"
	"fmt"

	"gorm.io/gorm"

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

// SourceToStore serializes inventory and maps associations for persistence.
func SourceToStore(s svcmodel.Source) (model.Source, error) {
	result := model.Source{
		ID:          s.ID,
		Name:        s.Name,
		VCenterID:   s.VCenterID,
		Username:    s.Username,
		OrgID:       s.OrgID,
		OnPremises:  s.OnPremises,
		UpdateType:  s.UpdateType,
		EmailDomain: s.EmailDomain,
		Model:       gorm.Model{CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt},
		ImageInfra:  ImageInfraToStore(s.ImageInfra),
	}
	if s.Inventory != nil {
		raw, err := marshalSourceInventory(*s.Inventory, s.Version)
		if err != nil {
			return model.Source{}, fmt.Errorf("failed to encode source %s inventory: %w", s.ID, err)
		}
		result.Inventory = raw
	}
	if s.Agents != nil {
		result.Agents = make([]model.Agent, len(s.Agents))
		for i, agent := range s.Agents {
			result.Agents[i] = AgentToStore(agent)
		}
	}
	if s.Labels != nil {
		result.Labels = make([]model.Label, len(s.Labels))
		for i, label := range s.Labels {
			result.Labels[i] = LabelToStore(label)
		}
	}
	return result, nil
}

// SourceSubsetInventoryToStore serializes a domain subset inventory.
func SourceSubsetInventoryToStore(s svcmodel.SourceSubsetInventory) (model.SourceSubsetInventory, error) {
	raw, err := marshalSourceInventory(s.Inventory, s.Version)
	if err != nil {
		return model.SourceSubsetInventory{}, fmt.Errorf("failed to encode subset %q inventory: %w", s.Name, err)
	}
	return model.SourceSubsetInventory{
		ID:         s.ID,
		Name:       s.Name,
		SourceID:   s.SourceID,
		VCenterID:  s.VCenterID,
		VMsCount:   s.VMsCount,
		UpdateType: s.UpdateType,
		Model:      gorm.Model{CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt},
		Inventory:  raw,
	}, nil
}

// AgentToStore converts source metadata to its persistence representation.
func AgentToStore(s svcmodel.Agent) model.Agent {
	return model.Agent{
		ID:         s.ID,
		Status:     s.Status,
		StatusInfo: s.StatusInfo,
		CredUrl:    s.CredUrl,
		Version:    s.Version,
		SourceID:   s.SourceID,
		Model:      gorm.Model{CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt},
	}
}

// ImageInfraToStore converts source metadata to its persistence representation.
func ImageInfraToStore(s svcmodel.ImageInfra) model.ImageInfra {
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
		Model:            gorm.Model{CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt, ID: s.ID},
	}
}

// LabelToStore converts source metadata to its persistence representation.
func LabelToStore(s svcmodel.Label) model.Label {
	return model.Label{
		Key:      s.Key,
		Value:    s.Value,
		SourceID: s.SourceID,
	}
}

func marshalSourceInventory(inventory svcmodel.Inventory, version uint) ([]byte, error) {
	if version == svcmodel.InventoryVersionV1 {
		return json.Marshal(inventory.Vcenter)
	}
	return json.Marshal(inventory)
}
