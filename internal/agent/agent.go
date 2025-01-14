package agent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/agent/client"
	"github.com/kubev2v/migration-planner/internal/agent/common"
	"github.com/kubev2v/migration-planner/internal/agent/config"
	"github.com/kubev2v/migration-planner/internal/agent/service"
	"github.com/lthibault/jitterbug"
	"go.uber.org/zap"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

const (
	defaultAgentPort = 3333
)

// This varible is set during build time.
// It contains the version of the code.
// For more info take a look into Makefile.
var version string

// New creates a new agent.
func New(id uuid.UUID, jwt string, config *config.Config) *Agent {
	return &Agent{
		config:            config,
		healthCheckStopCh: make(chan chan any),
		id:                id,
		jwt:               jwt,
	}
}

type Agent struct {
	config            *config.Config
	server            *Server
	healthCheckStopCh chan chan any
	credUrl           string
	id                uuid.UUID
	jwt               string
}

func (a *Agent) Run(ctx context.Context) error {
	var err error
	zap.S().Infof("Starting agent: %s", version)
	defer zap.S().Infof("Agent stopped")
	zap.S().Infof("Configuration: %s", a.config.String())

	defer utilruntime.HandleCrash()

	client, err := newPlannerClient(a.config)
	if err != nil {
		return err
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	ctx, cancel := context.WithCancel(ctx)
	a.start(ctx, client)

	select {
	case <-sig:
	case <-ctx.Done():
	}

	zap.S().Info("stopping agent...")

	a.Stop()
	cancel()

	return nil
}

func (a *Agent) Stop() {
	serverCh := make(chan any)
	a.server.Stop(serverCh)

	<-serverCh
	zap.S().Info("server stopped")

	c := make(chan any)
	a.healthCheckStopCh <- c
	<-c
	zap.S().Info("health check stopped")
}

func (a *Agent) start(ctx context.Context, plannerClient client.Planner) {
	inventoryUpdater := service.NewInventoryUpdater(a.id, plannerClient)
	statusUpdater := service.NewStatusUpdater(a.id, version, a.credUrl, a.config, plannerClient)

	// insert jwt into the context if any
	if a.jwt != "" {
		ctx = context.WithValue(ctx, common.JwtKey, a.jwt)
	}

	// start server
	a.server = NewServer(defaultAgentPort, a.config)
	go a.server.Start(statusUpdater)

	// get the credentials url
	credUrl := a.initializeCredentialUrl()

	// start the health check
	healthChecker, err := service.NewHealthChecker(
		plannerClient,
		a.config.DataDir,
		time.Duration(a.config.HealthCheckInterval*int64(time.Second)),
	)
	if err != nil {
		zap.S().Fatalf("failed to start health check: %w", err)
	}

	// TODO refactor health checker to call it from the main goroutine
	healthChecker.Start(ctx, a.healthCheckStopCh)

	collector := service.NewCollector(a.config.DataDir)
	collector.Collect(ctx)

	updateTicker := jitterbug.New(time.Duration(a.config.UpdateInterval.Duration), &jitterbug.Norm{Stdev: 30 * time.Millisecond, Mean: 0})

	/*
		Main loop
		The status of agent is always computed even if we don't have connectivity with the backend.
		If we're connected to the backend, the agent sends its status and if the status is UpToDate,
		it sends the inventory.
		In case of "source gone", it stops everything and break from the loop.
	*/
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-updateTicker.C:
			}

			// calculate status regardless if we have connectivity withe the backend.
			status, statusInfo, inventory := statusUpdater.CalculateStatus()

			//	check for health. Send requests only if we have connectivity
			if healthChecker.State() == service.HealthCheckStateConsoleUnreachable {
				continue
			}

			nsc := service.NewSmartStateClient()
			zap.S().Debug("trying to get smart state results")
			smartResults, err := nsc.GetResults()
			if err != nil {
				zap.S().Errorf("error getting smart analysis results: %s", err)
			}
			if smartResults != nil {
				inventory.SmartState = &smartResults
			}

			if err := statusUpdater.UpdateStatus(ctx, status, statusInfo, credUrl); err != nil {
				if errors.Is(err, client.ErrSourceGone) {
					zap.S().Info("Source is gone..Stop sending requests")
					// stop the server and the healthchecker
					a.Stop()
					break
				}
				zap.S().Errorf("unable to update agent status: %s", err)
				continue // skip inventory update if we cannot update agent's state.
			}

			if status == api.AgentStatusUpToDate {
				inventoryUpdater.UpdateServiceWithInventory(ctx, inventory)
			}
		}
	}()
}

func (a *Agent) initializeCredentialUrl() string {
	// Parse the service URL
	parsedURL, err := url.Parse(a.config.PlannerService.Service.Server)
	if err != nil {
		zap.S().Errorf("error parsing service URL: %v", err)
		return "N/A"
	}

	// Use either port if specified, or scheme
	port := parsedURL.Port()
	if port == "" {
		port = parsedURL.Scheme
	}

	// Connect to service
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%s", parsedURL.Hostname(), port))
	if err != nil {
		zap.S().Errorf("failed connecting to migration planner: %v", err)
		return "N/A"
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.TCPAddr)
	credUrl := fmt.Sprintf("http://%s:%d", localAddr.IP.String(), defaultAgentPort)
	zap.S().Infof("Discovered Agent IP address: %s", credUrl)
	return credUrl
}

func newPlannerClient(cfg *config.Config) (client.Planner, error) {
	httpClient, err := client.NewFromConfig(&cfg.PlannerService.Config)
	if err != nil {
		return nil, err
	}
	return client.NewPlanner(httpClient), nil
}
