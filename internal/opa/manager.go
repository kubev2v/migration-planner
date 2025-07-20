package opa

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	vspheremodel "github.com/kubev2v/forklift/pkg/controller/provider/model/vsphere"
	"go.uber.org/zap"
)

type Manager struct {
	server  *Server
	config  *Config
	running bool
}

type Config struct {
	Host           string
	Port           string
	PoliciesDir    string
	StartupTimeout time.Duration
}

func NewManager(policiesDir string) *Manager {
	return &Manager{
		config: &Config{
			Host:           "127.0.0.1",
			Port:           "8181",
			PoliciesDir:    policiesDir,
			StartupTimeout: 60 * time.Second, // Increased for loaded environments
		},
	}
}

func (m *Manager) Initialize() error {
	if !isPoliciesDirectory(m.config.PoliciesDir) {
		return fmt.Errorf("policies directory does not exist or contains no .rego files: %s", m.config.PoliciesDir)
	}

	zap.S().Named("opa").Infof("Using policies directory: %s", m.config.PoliciesDir)

	m.server = NewServer(m.config)
	if err := m.server.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize OPA server: %w", err)
	}

	m.running = true
	zap.S().Named("opa").Infof("OPA manager initialized successfully")
	return nil
}

func (m *Manager) IsRunning() bool {
	return m.running
}

func (m *Manager) ValidateVMs(vms *[]vspheremodel.VM) (*[]vspheremodel.VM, error) {
	if !m.IsRunning() {
		return vms, fmt.Errorf("OPA manager is not running")
	}

	return m.server.ValidateVMs(vms)
}

func (m *Manager) Shutdown() {
	if m.server != nil {
		m.server.Shutdown()
	}
	m.running = false
	zap.S().Named("opa").Info("OPA manager shut down")
}

func isPoliciesDirectory(dir string) bool {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return false
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.rego"))
	if err != nil {
		return false
	}

	return len(files) > 0
}
