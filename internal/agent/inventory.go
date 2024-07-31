package agent

import (
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
	log     *log.PrefixLogger
	config  *Config
	client  client.Planner
	credUrl string
}

func NewInventoryUpdater(log *log.PrefixLogger, config *Config, client client.Planner) *InventoryUpdater {
	return &InventoryUpdater{
		log:    log,
		config: config,
		client: client}
}

func (u *InventoryUpdater) UpdateServiceWithInventory(ctx context.Context) {
	updateTicker := jitterbug.New(time.Duration(u.config.UpdateInterval.Duration), &jitterbug.Norm{Stdev: 30 * time.Millisecond, Mean: 0})
	defer updateTicker.Stop()

	u.initializeCredentialUrl()
	inventoryFilePath := filepath.Join(u.config.DataDir, InventoryFile)
	credentialsFilePath := filepath.Join(u.config.DataDir, CredentialsFile)
	reader := fileio.NewReader()

	for {
		select {
		case <-ctx.Done():
			return
		case <-updateTicker.C:
			err := reader.CheckPathExists(credentialsFilePath)
			if err != nil {
				u.updateSourceStatus(ctx, api.SourceStatusWaitingForCredentials, "", "")
				continue
			}
			err = reader.CheckPathExists(inventoryFilePath)
			if err != nil {
				u.updateSourceStatus(ctx, api.SourceStatusGatheringInitialInventory, "", "")
				continue
			}
			inventoryData, err := reader.ReadFile(inventoryFilePath)
			if err != nil {
				u.updateSourceStatus(ctx, api.SourceStatusError, fmt.Sprintf("failed reading inventory file: %v", err), "")
				continue
			}
			var inventory InventoryData
			err = json.Unmarshal(inventoryData, &inventory)
			if err != nil {
				u.updateSourceStatus(ctx, api.SourceStatusError, fmt.Sprintf("invalid inventory file: %v", err), "")
				continue
			}
			newStatus := api.SourceStatusUpToDate
			if len(inventory.Error) > 0 {
				newStatus = api.SourceStatusError
			}
			u.updateSourceStatus(ctx, newStatus, inventory.Error, inventory.Inventory)
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
	u.credUrl = fmt.Sprintf("http://%s:%s", localAddr.IP.String(), u.config.CredUIPort)
}

func (u *InventoryUpdater) updateSourceStatus(ctx context.Context, status api.SourceStatus, statusInfo, inventory string) {
	update := agentapi.SourceStatusUpdate{
		Status:        string(status),
		StatusInfo:    statusInfo,
		Inventory:     inventory,
		CredentialUrl: u.credUrl,
	}
	u.log.Debugf("Updating status to %s: %s", string(status), statusInfo)
	err := u.client.UpdateSourceStatus(ctx, u.config.SourceID, update)
	if err != nil {
		u.log.Errorf("failed updating status: %v", err)
	}
}
