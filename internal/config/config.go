package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/IBM/sarama"
	"github.com/kubev2v/migration-planner/internal/util"
	"sigs.k8s.io/yaml"
)

const (
	appName = "migration-planner"
)

type Config struct {
	Database *dbConfig  `json:"database,omitempty"`
	Service  *svcConfig `json:"service,omitempty"`
}

type dbConfig struct {
	Type     string `json:"type,omitempty"`
	Hostname string `json:"hostname,omitempty"`
	Port     uint   `json:"port,omitempty"`
	Name     string `json:"name,omitempty"`
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
}

type svcConfig struct {
	Address              string      `json:"address,omitempty"`
	AgentEndpointAddress string      `json:"agentEndpointAddress,omitempty"`
	BaseUrl              string      `json:"baseUrl,omitempty"`
	BaseAgentEndpointUrl string      `json:"baseAgentEndpointUrl,omitempty"`
	LogLevel             string      `json:"logLevel,omitempty"`
	Kafka                kafkaConfig `json:"kafka,omitempty"`
}

type kafkaConfig struct {
	Brokers  []string            `yaml:"brokers"`
	Topic    string              `yaml:"topic"`
	Version  sarama.KafkaVersion `yaml:"-"`
	ClientID string              `yaml:"clientID"`

	SaramaConfig *sarama.Config
}

func ConfigDir() string {
	return filepath.Join(util.MustString(os.UserHomeDir), "."+appName)
}

func ConfigFile() string {
	return filepath.Join(ConfigDir(), "config.yaml")
}

func ClientConfigFile() string {
	return filepath.Join(ConfigDir(), "client.yaml")
}

func NewDefault() *Config {
	c := &Config{
		Database: &dbConfig{
			Type:     "pgsql",
			Hostname: "localhost",
			Port:     5432,
			Name:     "planner",
			User:     "admin",
			Password: "adminpass",
		},
		Service: &svcConfig{
			Address:              ":3443",
			AgentEndpointAddress: ":7443",
			BaseUrl:              "https://localhost:3443",
			BaseAgentEndpointUrl: "https://localhost:7443",
			LogLevel:             "info",
		},
	}
	return c
}

func NewFromFile(cfgFile string) (*Config, error) {
	cfg, err := Load(cfgFile)
	if err != nil {
		return nil, err
	}
	if err := Validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func LoadOrGenerate(cfgFile string) (*Config, error) {
	if _, err := os.Stat(cfgFile); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(cfgFile), os.FileMode(0755)); err != nil {
			return nil, fmt.Errorf("creating directory for config file: %v", err)
		}
		if err := Save(NewDefault(), cfgFile); err != nil {
			return nil, err
		}
	}
	return NewFromFile(cfgFile)
}

func Load(cfgFile string) (*Config, error) {
	contents, err := os.ReadFile(cfgFile)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %v", err)
	}
	c := &Config{}
	if err := yaml.Unmarshal(contents, c); err != nil {
		return nil, fmt.Errorf("decoding config: %v", err)
	}
	return c, nil
}

func Save(cfg *Config, cfgFile string) error {
	contents, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encoding config: %v", err)
	}
	if err := os.WriteFile(cfgFile, contents, 0600); err != nil {
		return fmt.Errorf("writing config file: %v", err)
	}
	return nil
}

func Validate(cfg *Config) error {
	return nil
}

func (cfg *Config) String() string {
	contents, err := json.Marshal(cfg) // nolint: staticcheck
	if err != nil {
		return "<error>"
	}
	return string(contents)
}
