package agent

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kubev2v/migration-planner/internal/agent/client"
	"github.com/kubev2v/migration-planner/pkg/log"
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
	var err error
	a.log.Infof("Starting agent...")
	defer a.log.Infof("Agent stopped")
	a.log.Infof("Configuration: %s", a.config.String())

	defer utilruntime.HandleCrash()
	ctx, cancel := context.WithCancel(ctx)
	shutdownSignals := []os.Signal{os.Interrupt, syscall.SIGTERM}
	// handle teardown
	shutdownHandler := make(chan os.Signal, 2)
	signal.Notify(shutdownHandler, shutdownSignals...)
	// health check closing ch
	healthCheckCh := make(chan chan any)
	go func(ctx context.Context) {
		select {
		case <-shutdownHandler:
			a.log.Infof("Received SIGTERM or SIGINT signal, shutting down.")
			//We must wait for the health checker to close any open requests and the log file.
			c := make(chan any)
			healthCheckCh <- c
			<-c
			a.log.Infof("Health check stopped.")

			close(shutdownHandler)
			cancel()
		case <-ctx.Done():
			a.log.Infof("Context has been cancelled, shutting down.")
			//We must wait for the health checker to close any open requests and the log file.
			c := make(chan any)
			healthCheckCh <- c
			<-c
			a.log.Infof("Health check stopped.")

			close(shutdownHandler)
			cancel()
		}
	}(ctx)

	StartServer(a.log, a.config)

	client, err := newPlannerClient(a.config)
	if err != nil {
		return err
	}

	// start the health check
	healthChecker, err := NewHealthChecker(
		a.log,
		client,
		a.config.DataDir,
		time.Duration(a.config.HealthCheckInterval*int64(time.Second)),
	)
	if err != nil {
		return err
	}
	healthChecker.Start(healthCheckCh)

	collector := NewCollector(a.log, a.config.DataDir)
	collector.collect(ctx)

	inventoryUpdater := NewInventoryUpdater(a.log, a.config, client)
	inventoryUpdater.UpdateServiceWithInventory(ctx)

	return nil
}

func newPlannerClient(cfg *Config) (client.Planner, error) {
	httpClient, err := client.NewFromConfig(&cfg.PlannerService.Config)
	if err != nil {
		return nil, err
	}
	return client.NewPlanner(httpClient), nil
}
