package service

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	agentapi "github.com/kubev2v/migration-planner/api/v1alpha1/agent"
	"github.com/kubev2v/migration-planner/internal/agent/client"
	"github.com/kubev2v/migration-planner/internal/agent/config"
	"github.com/kubev2v/migration-planner/internal/agent/fileio"
)

const (
	defaultUpdateStatusTimeout = 5 * time.Second
)

type StatusUpdater struct {
	AgentID  uuid.UUID
	sourceID uuid.UUID
	version  string
	config   *config.Config
	client   client.Planner
	credUrl  string
}

func NewStatusUpdater(sourceID, agentID uuid.UUID, version, credUrl string, config *config.Config, client client.Planner) *StatusUpdater {
	return &StatusUpdater{
		client:   client,
		config:   config,
		AgentID:  agentID,
		sourceID: sourceID,
		credUrl:  credUrl,
		version:  version,
	}
}

func (s *StatusUpdater) UpdateStatus(ctx context.Context, status api.AgentStatus, statusInfo string, credUrl string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultUpdateStatusTimeout*time.Second)
	defer cancel()

	bodyParameters := agentapi.AgentStatusUpdate{
		Status:        string(status),
		StatusInfo:    statusInfo,
		CredentialUrl: credUrl,
		Version:       s.version,
		SourceId:      s.sourceID,
	}

	err := s.client.UpdateAgentStatus(ctx, s.AgentID, bodyParameters)
	if err != nil {
		return err
	}

	return nil
}

func (s *StatusUpdater) CalculateStatus() (api.AgentStatus, string, *api.Inventory) {
	inventoryFilePath := filepath.Join(s.config.DataDir, config.InventoryFile)
	credentialsFilePath := filepath.Join(s.config.PersistentDataDir, config.CredentialsFile)
	reader := fileio.NewReader()

	err := reader.CheckPathExists(credentialsFilePath)
	if err != nil {
		return api.AgentStatusWaitingForCredentials, "No credentials provided", nil
	}
	err = reader.CheckPathExists(inventoryFilePath)
	if err != nil {
		return api.AgentStatusGatheringInitialInventory, "Inventory not yet collected", nil
	}
	inventoryData, err := reader.ReadFile(inventoryFilePath)
	if err != nil {
		return api.AgentStatusError, fmt.Sprintf("Failed reading inventory file: %v", err), nil
	}
	var inventory InventoryData
	err = json.Unmarshal(inventoryData, &inventory)
	if err != nil {
		return api.AgentStatusError, fmt.Sprintf("Invalid inventory file: %v", err), nil
	}
	if len(inventory.Error) > 0 {
		return api.AgentStatusError, inventory.Error, &inventory.Inventory
	}
	return api.AgentStatusUpToDate, "Inventory successfully collected", &inventory.Inventory
}
