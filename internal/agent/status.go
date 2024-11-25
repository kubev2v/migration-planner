package agent

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
	"github.com/kubev2v/migration-planner/internal/agent/fileio"
	"github.com/kubev2v/migration-planner/pkg/log"
)

const (
	defaultUpdateStatusTimeout = 5 * time.Second
)

type StatusUpdater struct {
	agentID uuid.UUID
	log     *log.PrefixLogger
	version string
	config  *Config
	client  client.Planner
	credUrl string
}

func NewStatusUpdater(log *log.PrefixLogger, agentID uuid.UUID, version, credUrl string, config *Config, client client.Planner) *StatusUpdater {
	return &StatusUpdater{
		log:     log,
		client:  client,
		config:  config,
		agentID: agentID,
		credUrl: credUrl,
		version: version,
	}
}

func (s *StatusUpdater) UpdateStatus(ctx context.Context, status api.AgentStatus, statusInfo string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultUpdateStatusTimeout*time.Second)
	defer cancel()

	bodyParameters := agentapi.AgentStatusUpdate{
		Id:            s.agentID.String(),
		Status:        string(status),
		StatusInfo:    statusInfo,
		CredentialUrl: s.credUrl,
		Version:       s.version,
	}

	return s.client.UpdateAgentStatus(ctx, s.agentID, bodyParameters)
}

func (s *StatusUpdater) CalculateStatus() (api.AgentStatus, string, *api.Inventory) {
	inventoryFilePath := filepath.Join(s.config.DataDir, InventoryFile)
	credentialsFilePath := filepath.Join(s.config.DataDir, CredentialsFile)
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
