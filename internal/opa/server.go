package opa

import (
	"context"
	"fmt"
	"net/http"
	"time"

	vspheremodel "github.com/kubev2v/forklift/pkg/controller/provider/model/vsphere"
	"github.com/kubev2v/migration-planner/internal/agent/collector"
	"github.com/open-policy-agent/opa/runtime"
	"go.uber.org/zap"
)

// Server handles the OPA runtime server
type Server struct {
	config  *Config
	runtime *runtime.Runtime
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
}

// NewServer creates a new OPA server instance
func NewServer(config *Config) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		config: config,
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
	}
}

// Initialize starts the OPA runtime server
func (s *Server) Initialize() error {
	// Start OPA runtime server
	if err := s.startOPARuntime(); err != nil {
		return fmt.Errorf("failed to start OPA runtime: %w", err)
	}

	// Wait for OPA server to be ready
	if err := s.waitForServer(); err != nil {
		s.Shutdown()
		return fmt.Errorf("OPA runtime failed to start: %w", err)
	}

	zap.S().Named("opa").Infof("OPA runtime started successfully on %s:%s", s.config.Host, s.config.Port)
	return nil
}

// startOPARuntime starts the OPA runtime server
func (s *Server) startOPARuntime() error {
	serverAddr := fmt.Sprintf("%s:%s", s.config.Host, s.config.Port)

	params := runtime.Params{
		Addrs: &[]string{serverAddr},
		Paths: []string{s.config.PoliciesDir},
	}

	rt, err := runtime.NewRuntime(s.ctx, params)
	if err != nil {
		return fmt.Errorf("failed to create OPA runtime: %w", err)
	}

	s.runtime = rt
	zap.S().Named("opa").Infof("Starting OPA runtime server on %s with policies from %s", serverAddr, s.config.PoliciesDir)

	// Start the server in a goroutine
	go func() {
		defer close(s.done) // Signal when goroutine finishes
		s.runtime.StartServer(s.ctx)
		zap.S().Named("opa").Info("OPA runtime server stopped")
	}()

	return nil
}

// waitForServer waits for OPA server to be ready
func (s *Server) waitForServer() error {
	opaServer := fmt.Sprintf("%s:%s", s.config.Host, s.config.Port)

	ticker := time.NewTicker(5 * time.Second) // Check every 5 seconds to reduce load during startup
	defer ticker.Stop()

	timeout := time.NewTimer(s.config.StartupTimeout)
	defer timeout.Stop()

	for {
		select {
		case <-timeout.C:
			return fmt.Errorf("OPA server did not start within %v", s.config.StartupTimeout)
		case <-ticker.C:
			if s.isOPAServerAlive(opaServer) {
				return nil
			}
		}
	}
}

// isOPAServerAlive checks if OPA server is responding
func (s *Server) isOPAServerAlive(opaServer string) bool {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://" + opaServer + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

// ValidateVMs validates VMs using the OPA server via HTTP
func (s *Server) ValidateVMs(vms *[]vspheremodel.VM) (*[]vspheremodel.VM, error) {
	opaServer := fmt.Sprintf("%s:%s", s.config.Host, s.config.Port)

	// Use existing HTTP-based validation function
	result, err := collector.Validation(vms, opaServer)
	if err != nil {
		return vms, err
	}

	return result, nil
}

// Shutdown gracefully shuts down the OPA server
func (s *Server) Shutdown() {
	if s.runtime != nil {
		zap.S().Named("opa").Info("Stopping OPA runtime server")
		s.cancel() // Signal the server to stop

		// Wait for the server goroutine to finish
		select {
		case <-s.done:
			zap.S().Named("opa").Info("OPA runtime server shutdown complete")
		case <-time.After(5 * time.Second):
			zap.S().Named("opa").Warn("OPA runtime server shutdown timed out after 5 seconds")
		}
	}
}
