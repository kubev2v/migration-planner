package agent

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	api "github.com/kubev2v/migration-planner/api/v1alpha1/agent"
	"github.com/kubev2v/migration-planner/internal/agent/client"
	"github.com/kubev2v/migration-planner/internal/agent/fileio"
	"github.com/kubev2v/migration-planner/pkg/log"
	"github.com/lthibault/jitterbug"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

const (
	// name of the file which stores the current inventory
	InventoryFile = "inventory.json"
)

// New creates a new agent.
func New(log *log.PrefixLogger, config *Config) *Agent {
	return &Agent{
		config: config,
		log:    log,
	}
}

type Agent struct {
	config *Config
	log    *log.PrefixLogger
}

func (a *Agent) GetLogPrefix() string {
	return a.log.Prefix()
}

func (a *Agent) Run(ctx context.Context) error {
	a.log.Infof("Starting agent...")
	defer a.log.Infof("Agent stopped")

	defer utilruntime.HandleCrash()
	ctx, cancel := context.WithCancel(ctx)
	shutdownSignals := []os.Signal{os.Interrupt, syscall.SIGTERM}

	// handle teardown
	shutdownHandler := make(chan os.Signal, 2)
	signal.Notify(shutdownHandler, shutdownSignals...)
	go func(ctx context.Context) {
		select {
		case <-shutdownHandler:
			a.log.Infof("Received SIGTERM or SIGINT signal, shutting down.")
			close(shutdownHandler)
			cancel()
		case <-ctx.Done():
			a.log.Infof("Context has been cancelled, shutting down.")
			close(shutdownHandler)
			cancel()
		}
	}(ctx)

	reader := initializeFileIO(a.config)
	inventoryFilePath := filepath.Join(a.config.DataDir, InventoryFile)

	plannerClient, err := newPlannerClient(a.config)
	if err != nil {
		return err
	}

	updateInventoryTicker := jitterbug.New(time.Duration(a.config.InventoryUpdateInterval), &jitterbug.Norm{Stdev: 30 * time.Millisecond, Mean: 0})
	defer updateInventoryTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-updateInventoryTicker.C:
			inventoryData, err := reader.ReadFile(inventoryFilePath)
			if err != nil {
				a.log.Errorf("failed reading inventory file: %v", err)
				continue
			}
			err = plannerClient.UpdateSourceInventory(ctx, a.config.SourceID, api.SourceInventoryUpdate{Inventory: string(inventoryData)})
			if err != nil {
				a.log.Errorf("failed updating inventory: %v", err)
				continue
			}
		}
	}
}

func newPlannerClient(cfg *Config) (client.Planner, error) {
	httpClient, err := client.NewFromConfig(&cfg.PlannerService.Config)
	if err != nil {
		return nil, err
	}
	return client.NewPlanner(httpClient), nil
}

func initializeFileIO(cfg *Config) *fileio.Reader {
	deviceReader := fileio.NewReader()
	return deviceReader
}
