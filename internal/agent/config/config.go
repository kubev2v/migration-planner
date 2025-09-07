package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/kubev2v/migration-planner/internal/agent/client"
	"github.com/kubev2v/migration-planner/internal/agent/fileio"
	"github.com/kubev2v/migration-planner/internal/util"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/yaml"
)

const (
	// name of the file which stores the source credentials
	CredentialsFile = "credentials.json"
	// name of the file which stores the current inventory
	InventoryFile = "inventory.json"
	// DefaultUpdateInterval is the default interval between two status updates
	DefaultUpdateInterval = time.Duration(60 * time.Second)
	// DefaultConfigDir is the default directory where the device's configuration is stored
	DefaultConfigDir = "/etc/planner"
	// DefaultConfigFile is the default path to the agent's configuration file
	DefaultConfigFile = DefaultConfigDir + "/config.yaml"
	// DefaultDataDir is the default directory where the agent's data is stored
	DefaultDataDir = ".migration-planner/data"
	// DefaultPersistentDataDir is the default directory where the agent's data is stored
	DefaultPersistentDataDir = ".migration-planner/persistent-data"
	// DefaultWwwDir is the default directory from which the agent serves static files
	DefaultWwwDir = "/app/www"
	// DefaultOpaPoliciesDir is the default directory where the OPA policies are stored
	DefaultOpaPoliciesDir = "./policies"
	// DefaultPlannerEndpoint is the default address of the migration planner server
	DefaultPlannerEndpoint = "https://localhost:7443"
	// DefaultHealthCheck is the default value for health check interval in seconds.
	// default value set 10s health check should be faster than the update period in order to block it
	// if the console is unreachable
	DefaultHealthCheck = 10
)

var (
	DefaultSourceId = uuid.Nil.String()
)

type Credentials struct {
	URL                  string `json:"url"`
	Username             string `json:"username"`
	Password             string `json:"password"`
	IsDataSharingAllowed bool   `json:"isDataSharingAllowed"`
}

type Config struct {
	// ConfigDir is the directory where the agent's configuration is stored
	ConfigDir string `json:"config-dir"`
	// DataDir is the directory where the agent's data is stored
	DataDir string `json:"data-dir"`
	// PersistentDataDir is the directory where persistent data is stored
	PersistentDataDir string `json:"persistent-data-dir"`
	// WwwDir is the directory from which the agent serves static files
	WwwDir string `json:"www-dir"`
	// OpaPoliciesDir is the path to the directory containing OPA policy files
	OpaPoliciesDir string `json:"policies-dir"`

	// SourceID is the ID of this source in the planner
	SourceID string `json:"source-id"`

	// PlannerService is the client configuration for connecting to the migration planner server
	PlannerService PlannerService `json:"planner-service,omitempty"`

	// UpdateInterval is the interval between two status updates
	UpdateInterval util.Duration `json:"update-interval,omitempty"`

	// LogLevel is the level of logging. can be:  "panic", "fatal", "error", "warn"/"warning",
	// "info", "debug" or "trace", any other will be treated as "info"
	LogLevel string `json:"log-level,omitempty"`
	// LogPrefix is the log prefix used for testing
	LogPrefix string `json:"log-prefix,omitempty"`

	reader *fileio.Reader
}

type PlannerService struct {
	client.Config
}

func (s *PlannerService) Equal(s2 *PlannerService) bool {
	if s == s2 {
		return true
	}
	return s.Config.Equal(&s2.Config)
}

func NewDefault() *Config {
	opaPoliciesDir := DefaultOpaPoliciesDir
	// Override with environment variable if set when running locally
	if envDir := os.Getenv("MIGRATION_PLANNER_OPA_POLICIES_FOLDER"); envDir != "" {
		opaPoliciesDir = envDir
	}

	c := &Config{
		ConfigDir:         DefaultConfigDir,
		DataDir:           DefaultDataDir,
		PersistentDataDir: DefaultPersistentDataDir,
		WwwDir:            DefaultWwwDir,
		OpaPoliciesDir:    opaPoliciesDir,
		PlannerService:    PlannerService{Config: *client.NewDefault()},
		UpdateInterval:    util.Duration{Duration: DefaultUpdateInterval},
		reader:            fileio.NewReader(),
		LogLevel:          logrus.InfoLevel.String(),
	}

	return c
}

// Validate checks that the required fields are set and that the paths exist.
func (cfg *Config) Validate() error {
	if err := cfg.PlannerService.Validate(); err != nil {
		return err
	}
	requiredFields := []struct {
		value     string
		name      string
		checkPath bool
	}{
		{cfg.DataDir, "data-dir", true},
		{cfg.PersistentDataDir, "persistent-data-dir", true},
	}

	for _, field := range requiredFields {
		if field.value == "" {
			return fmt.Errorf("%s is required", field.name)
		}
		if field.checkPath {
			if err := cfg.reader.CheckPathExists(field.value); err != nil {
				return fmt.Errorf("%s: %w", field.name, err)
			}
		}
	}

	return nil
}

// ParseConfigFile reads the config file and unmarshals it into the Config struct
func (cfg *Config) ParseConfigFile(cfgFile string) error {
	contents, err := cfg.reader.ReadFile(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}
	if err := yaml.Unmarshal(contents, cfg); err != nil {
		return fmt.Errorf("failed to unmarshal config file: %w", err)
	}
	cfg.PlannerService.Config.SetBaseDir(filepath.Dir(cfgFile))
	return nil
}

func (cfg *Config) String() string {
	contents, err := json.Marshal(cfg)
	if err != nil {
		return "<error>"
	}
	return string(contents)
}

func (cfg *Config) CreateDefaultDirs() error {
	base, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("error getting user home directory: %v", err)
	}

	cfg.PersistentDataDir = filepath.Join(base, cfg.PersistentDataDir)
	cfg.DataDir = filepath.Join(base, cfg.DataDir)

	for _, dir := range []string{cfg.PersistentDataDir, cfg.DataDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}
	return nil
}
