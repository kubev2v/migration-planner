package service

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/google/uuid"
	vsphere "github.com/kubev2v/forklift/pkg/controller/provider/model/vsphere"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	agentapi "github.com/kubev2v/migration-planner/api/v1alpha1/agent"
	"github.com/kubev2v/migration-planner/internal/agent/client"
	"go.uber.org/zap"
)

type InventoryUpdater struct {
	client     client.Planner
	agentID    uuid.UUID
	sourceID   uuid.UUID
	prevStatus []byte
}

type InventoryData struct {
	Inventory api.Inventory `json:"inventory"`
	Error     string        `json:"error"`
}

// InfrastructureData contains all the infrastructure-related data needed to create an inventory
type InfrastructureData struct {
	Datastores            []api.Datastore
	Networks              []api.Network
	HostPowerStates       map[string]int
	Hosts                 *[]api.Host
	HostsPerCluster       []int
	ClustersPerDatacenter []int
	TotalHosts            int
	TotalClusters         int
	TotalDatacenters      int
	VmsPerCluster         []int
}

// CreateBasicInventory creates a basic inventory object with the provided data
// This function consolidates the duplicated createBasicInventoryObj functions from
// vsphere.go and parser.go to ensure consistency and reduce duplication.
func CreateBasicInventory(
	vCenterID string,
	vms *[]vsphere.VM,
	infraData InfrastructureData,
) *api.Inventory {
	return &api.Inventory{
		Vcenter: api.VCenter{
			Id: vCenterID,
		},
		Vms: api.VMs{
			Total:                len(*vms),
			PowerStates:          map[string]int{},
			Os:                   map[string]int{},
			OsInfo:               &map[string]api.OsInfo{},
			DiskSizeTier:         &map[string]api.DiskSizeTierSummary{},
			DiskTypes:            &map[string]api.DiskTypeSummary{},
			MigrationWarnings:    api.MigrationIssues{},
			NotMigratableReasons: api.MigrationIssues{},
			// TODO: refactor, hot fix for https://issues.redhat.com/browse/ECOPROJECT-3423
			CpuCores:  api.VMResourceBreakdown{Histogram: api.Histogram{Data: []int{}}},
			RamGB:     api.VMResourceBreakdown{Histogram: api.Histogram{Data: []int{}}},
			DiskCount: api.VMResourceBreakdown{Histogram: api.Histogram{Data: []int{}}},
			DiskGB:    api.VMResourceBreakdown{Histogram: api.Histogram{Data: []int{}}},
			NicCount:  &api.VMResourceBreakdown{Histogram: api.Histogram{Data: []int{}}},
		},
		Infra: api.Infra{
			ClustersPerDatacenter: &infraData.ClustersPerDatacenter,
			Datastores:            infraData.Datastores,
			HostPowerStates:       infraData.HostPowerStates,
			Hosts:                 infraData.Hosts,
			TotalHosts:            infraData.TotalHosts,
			TotalClusters:         infraData.TotalClusters,
			TotalDatacenters:      &infraData.TotalDatacenters,
			HostsPerCluster:       infraData.HostsPerCluster,
			Networks:              infraData.Networks,
			VmsPerCluster:         &infraData.VmsPerCluster,
		},
	}
}

func NewInventoryUpdater(sourceID, agentID uuid.UUID, client client.Planner) *InventoryUpdater {
	updater := &InventoryUpdater{
		client:     client,
		agentID:    agentID,
		sourceID:   sourceID,
		prevStatus: []byte{},
	}
	return updater
}

func (u *InventoryUpdater) UpdateServiceWithInventory(ctx context.Context, inventory *api.Inventory) {
	update := agentapi.SourceStatusUpdate{
		Inventory: *inventory,
		AgentId:   u.agentID,
	}

	newContents, err := json.Marshal(update)
	if err != nil {
		zap.S().Named("inventory").Errorf("failed to marshal new status: %v", err)
	}
	if bytes.Equal(u.prevStatus, newContents) {
		zap.S().Named("inventory").Debug("Local status did not change, skipping service update")
		return
	}

	err = u.client.UpdateSourceStatus(ctx, u.sourceID, update)
	if err != nil {
		zap.S().Named("inventory").Errorf("failed to update status: %v", err)
		return
	}

	u.prevStatus = newContents
}
