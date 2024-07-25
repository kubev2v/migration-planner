package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	agentapi "github.com/kubev2v/migration-planner/api/v1alpha1/agent"
	"github.com/kubev2v/migration-planner/internal/agent/client"
	"github.com/kubev2v/migration-planner/internal/agent/fileio"
	"github.com/kubev2v/migration-planner/pkg/log"
	"github.com/lthibault/jitterbug"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

const (
	// name of the file which stores the current inventory
	InventoryFile = "inventory.json"

	// name of the file which stores the source credentials
	CredentialsFile = "credentials.json"
)

var sourceStatuses = []string{}

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
	client client.Planner
	reader *fileio.Reader
	writer *fileio.Writer
}

type InventoryData struct {
	Inventory string
	Error     string
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

	a.initializeFileIO()

	server := &http.Server{Addr: "0.0.0.0:3333", Handler: a.RESTService()}
	serverCtx, serverStopCtx := context.WithCancel(context.Background())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-sig
		shutdownCtx, _ := context.WithTimeout(serverCtx, 30*time.Second)

		go func() {
			<-shutdownCtx.Done()
			if shutdownCtx.Err() == context.DeadlineExceeded {
				a.log.Fatal("graceful shutdown timed out.. forcing exit.")
			}
		}()

		// Trigger graceful shutdown
		err := server.Shutdown(shutdownCtx)
		if err != nil {
			a.log.Fatal(err)
		}
		serverStopCtx()
	}()

	go func() {
		// Run the server
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			a.log.Fatal(err)
		}

		// Wait for server context to be stopped
		<-serverCtx.Done()
	}()

	updateTicker := jitterbug.New(time.Duration(a.config.UpdateInterval.Duration), &jitterbug.Norm{Stdev: 30 * time.Millisecond, Mean: 0})
	defer updateTicker.Stop()

	inventoryFilePath := filepath.Join(a.config.DataDir, InventoryFile)
	credentialsFilePath := filepath.Join(a.config.DataDir, CredentialsFile)
	a.client, err = newPlannerClient(a.config)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-updateTicker.C:
			err := a.reader.CheckPathExists(credentialsFilePath)
			if err != nil {
				a.updateSourceStatus(ctx, api.SourceStatusWaitingForCredentials, "", "")
				continue
			}
			err = a.reader.CheckPathExists(inventoryFilePath)
			if err != nil {
				a.updateSourceStatus(ctx, api.SourceStatusGatheringInitialInventory, "", "")
				continue
			}
			inventoryData, err := a.reader.ReadFile(inventoryFilePath)
			if err != nil {
				a.updateSourceStatus(ctx, api.SourceStatusError, fmt.Sprintf("failed reading inventory file: %v", err), "")
				continue
			}
			var inventory InventoryData
			err = json.Unmarshal(inventoryData, &inventory)
			if err != nil {
				a.updateSourceStatus(ctx, api.SourceStatusError, fmt.Sprintf("invalid inventory file: %v", err), "")
				continue
			}
			newStatus := api.SourceStatusUpToDate
			if len(inventory.Error) > 0 {
				newStatus = api.SourceStatusError
			}
			a.updateSourceStatus(ctx, newStatus, inventory.Error, inventory.Inventory)
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

func (a *Agent) initializeFileIO() {
	a.reader = fileio.NewReader()
	a.writer = fileio.NewWriter()
}

func (a *Agent) RESTService() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)

	r.Put("/credentials", func(w http.ResponseWriter, r *http.Request) {
		a.credentialHandler(w, r)
	})

	return r
}

func (a *Agent) credentialHandler(w http.ResponseWriter, r *http.Request) {
	credPath := filepath.Join(a.config.DataDir, CredentialsFile)
	err := a.writer.WriteStreamToFile(credPath, r.Body)
	if err != nil {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(err.Error()))
	}
	w.WriteHeader(204)
}

func (a *Agent) updateSourceStatus(ctx context.Context, status api.SourceStatus, statusInfo, inventory string) {
	update := agentapi.SourceStatusUpdate{
		Status:     string(status),
		StatusInfo: statusInfo,
		Inventory:  inventory,
	}
	a.log.Debugf("Updating status to %s: %s", string(status), statusInfo)
	err := a.client.UpdateSourceStatus(ctx, a.config.SourceID, update)
	if err != nil {
		a.log.Errorf("failed updating status: %v", err)
	}
}
