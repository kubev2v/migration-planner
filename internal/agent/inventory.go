package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"time"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	agentapi "github.com/kubev2v/migration-planner/api/v1alpha1/agent"
	"github.com/kubev2v/migration-planner/internal/agent/client"
	"github.com/kubev2v/migration-planner/internal/agent/fileio"
	"github.com/kubev2v/migration-planner/pkg/log"
	"github.com/lthibault/jitterbug"
	gateway "github.com/net-byte/go-gateway"
)

type InventoryUpdater struct {
	log        *log.PrefixLogger
	config     *Config
	client     client.Planner
	credUrl    string
	prevStatus []byte
}

type InventoryData struct {
	Inventory api.Inventory
	Error     string
}

func NewInventoryUpdater(log *log.PrefixLogger, config *Config, client client.Planner) *InventoryUpdater {
	return &InventoryUpdater{
		log:        log,
		config:     config,
		client:     client,
		prevStatus: []byte{},
	}
}

func (u *InventoryUpdater) UpdateServiceWithInventory(ctx context.Context) {
	updateTicker := jitterbug.New(time.Duration(u.config.UpdateInterval.Duration), &jitterbug.Norm{Stdev: 30 * time.Millisecond, Mean: 0})
	defer updateTicker.Stop()

	u.initializeCredentialUrl()

	for {
		select {
		case <-ctx.Done():
			return
		case <-updateTicker.C:
			status, statusInfo, inventory := calculateStatus(u.config.DataDir)
			u.updateSourceStatus(ctx, status, statusInfo, inventory)
		}
	}
}

func (u *InventoryUpdater) initializeCredentialUrl() {
	gw, err := gateway.DiscoverGatewayIPv4()
	if err != nil {
		u.log.Errorf("failed finding default GW: %v", err)
		u.credUrl = "N/A"
		return
	}

	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%s", gw.String(), "80"))
	if err != nil {
		u.log.Errorf("failed connecting to default GW: %v", err)
		u.credUrl = "N/A"
		return
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.TCPAddr)
	u.credUrl = fmt.Sprintf("https://%s:%s", localAddr.IP.String(), u.config.CredUIPort)
}

func calculateStatus(dataDir string) (api.SourceStatus, string, *api.Inventory) {
	inventoryFilePath := filepath.Join(dataDir, InventoryFile)
	credentialsFilePath := filepath.Join(dataDir, CredentialsFile)
	reader := fileio.NewReader()

	err := reader.CheckPathExists(credentialsFilePath)
	if err != nil {
		return api.SourceStatusWaitingForCredentials, "No credentials provided", nil
	}
	err = reader.CheckPathExists(inventoryFilePath)
	if err != nil {
		return api.SourceStatusGatheringInitialInventory, "Inventory not yet collected", nil
	}
	inventoryData, err := reader.ReadFile(inventoryFilePath)
	if err != nil {
		return api.SourceStatusError, fmt.Sprintf("Failed reading inventory file: %v", err), nil
	}
	var inventory InventoryData
	err = json.Unmarshal(inventoryData, &inventory)
	if err != nil {
		return api.SourceStatusError, fmt.Sprintf("Invalid inventory file: %v", err), nil
	}
	if len(inventory.Error) > 0 {
		return api.SourceStatusError, inventory.Error, &inventory.Inventory
	}
	return api.SourceStatusUpToDate, "Inventory successfully collected", &inventory.Inventory
}

func (u *InventoryUpdater) updateSourceStatus(ctx context.Context, status api.SourceStatus, statusInfo string, inventory *api.Inventory) {
	update := agentapi.SourceStatusUpdate{
		Status:        string(status),
		StatusInfo:    statusInfo,
		Inventory:     inventory,
		CredentialUrl: u.credUrl,
	}

	newContents, err := json.Marshal(update)
	if err != nil {
		u.log.Errorf("failed marshalling new status: %v", err)
	}
	if bytes.Equal(u.prevStatus, newContents) {
		u.log.Debug("Local status did not change, skipping service update")
		return
	}

	u.log.Debugf("Updating status to %s: %s", string(status), statusInfo)
	err = u.client.UpdateSourceStatus(ctx, u.config.SourceID, update)
	if err != nil {
		u.log.Errorf("failed updating status: %v", err)
		return
	}

	u.prevStatus = newContents
}
