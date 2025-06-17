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
	"github.com/kubev2v/migration-planner/internal/agent/collector"
	"github.com/kubev2v/migration-planner/internal/agent/common"
	"github.com/kubev2v/migration-planner/internal/agent/config"
	"github.com/kubev2v/migration-planner/internal/agent/service"
	"github.com/lthibault/jitterbug"
	"go.uber.org/zap"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

type closeChKeyType struct{}

const (
	defaultAgentPort = 3333
)

// This varible is set during build time.
// It contains the version of the code.
// For more info take a look into Makefile.
var (
	version    string
	closeChKey closeChKeyType
)

// New creates a new agent.
func New(id uuid.UUID, jwt string, config *config.Config) *Agent {
	return &Agent{
		config: config,
		id:     id,
		jwt:    jwt,
	}
}

type Agent struct {
	config  *config.Config
	server  *Server
	credUrl string
	id      uuid.UUID
	jwt     string
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

	closeCh := make(chan any, 1)
	ctx, cancel := context.WithCancel(context.WithValue(ctx, closeChKey, closeCh))
	a.start(ctx, client)

	select {
	case <-sig:
	case <-ctx.Done():
	}

	zap.S().Info("stopping agent...")

	a.Stop()
	cancel()
	<-closeCh // Need to wait for main loop to exit before return to os.

	return nil
}

func (a *Agent) Stop() {
	serverCh := make(chan any)
	a.server.Stop(serverCh)

	<-serverCh
	zap.S().Info("server stopped")
}

func (a *Agent) start(ctx context.Context, plannerClient client.Planner) {
	inventoryUpdater := service.NewInventoryUpdater(uuid.MustParse(a.config.SourceID), a.id, plannerClient)
	statusUpdater := service.NewStatusUpdater(uuid.MustParse(a.config.SourceID), a.id, version, a.credUrl, a.config, plannerClient)

	// insert jwt into the context if any
	if a.jwt != "" {
		ctx = context.WithValue(ctx, common.JwtKey, a.jwt)
	}

	// get the credentials url
	credUrl := a.initializeCredentialUrl()

	cert, key, err := NewSelfSignedCertificateProvider(credUrl).GetCertificate(time.Now().AddDate(1, 0, 0))
	if err != nil {
		zap.S().Named("agent").Errorf("failed to generate certificate: %s", err)
	}

	// start server
	a.server = NewServer(defaultAgentPort, a.config, cert, key)
	go a.server.Start(statusUpdater)

	protocol := "http"
	if a.server.tlsConfig != nil {
		protocol = "https"
	}

	a.credUrl = "N/A"
	if credUrl != nil {
		a.credUrl = fmt.Sprintf("%s://%s:%d", protocol, credUrl.IP.String(), defaultAgentPort)
	}
	zap.S().Infof("Discovered Agent IP address: %s", a.credUrl)

	c := collector.NewCollector(a.config.DataDir, a.config.PersistentDataDir)
	c.Collect(ctx)

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
				if ch, ok := ctx.Value(closeChKey).(chan any); ok {
					ch <- struct{}{}
				}
				return
			case <-updateTicker.C:
			}

			// calculate status regardless if we have connectivity with the backend.
			status, statusInfo, inventory := statusUpdater.CalculateStatus()

			if err := statusUpdater.UpdateStatus(ctx, status, statusInfo, a.credUrl); err != nil {
				if errors.Is(err, client.ErrSourceGone) {
					zap.S().Info("Source is gone..Stop sending requests")
					// stop the server and the healthchecker
					a.Stop()
					break
				}
				zap.S().Errorf("failed to update agent status: %s", err)
				continue // skip inventory update if we cannot update agent's state.
			}

			if status == api.AgentStatusUpToDate {
				inventoryUpdater.UpdateServiceWithInventory(ctx, inventory)
			}
		}
	}()
}

func (a *Agent) initializeCredentialUrl() *net.TCPAddr {
	// Parse the service URL
	parsedURL, err := url.Parse(a.config.PlannerService.Service.Server)
	if err != nil {
		zap.S().Errorf("falied to parse service URL: %v", err)
		return nil
	}

	// Use either port if specified, or scheme
	port := parsedURL.Port()
	if port == "" {
		port = parsedURL.Scheme
	}

	// Connect to service
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%s", parsedURL.Hostname(), port))
	if err != nil {
		zap.S().Errorf("failed to connect the migration planner: %v", err)
		return nil
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.TCPAddr)
	return localAddr
}

func newPlannerClient(cfg *config.Config) (client.Planner, error) {
	httpClient, err := client.NewFromConfig(&cfg.PlannerService.Config)
	if err != nil {
		return nil, err
	}
	return client.NewPlanner(httpClient), nil
}
