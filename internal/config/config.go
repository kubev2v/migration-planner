package config

import (
	"github.com/IBM/sarama"
	"github.com/kelseyhightower/envconfig"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/store"
)

const (
	appName         = "migration-planner"
	EnvConfigPrefix = "MIGRATION_PLANNER"
)

var singleConfig *Config = nil

type Config struct {
	Auth  auth.Config
	DB    store.Config
	Svc   SvcConfig
	Kafka KafkaConfig
}

type SvcConfig struct {
	Address              string `envconfig:"MIGRATION_PLANNER_ADDRESS" default:":3443"`
	AgentEndpointAddress string `envconfig:"MIGRATION_PLANNER_AGENT_ENDPOINT_ADDRESS" default:":7443"`
	ImageEndpointAddress string `envconfig:"MIGRATION_PLANNER_IMAGE_ENDPOINT_ADDRESS" default:":11443"`
	BaseUrl              string `envconfig:"MIGRATION_PLANNER_BASE_URL" default:"https://localhost:3443"`
	BaseAgentEndpointUrl string `envconfig:"MIGRATION_PLANNER_BASE_AGENT_ENDPOINT_URL" default:"https://localhost:7443"`
	BaseImageEndpointUrl string `envconfig:"MIGRATION_PLANNER_BASE_IMAGE_ENDPOINT_URL" default:"https://localhost:11443"`
	LogLevel             string `envconfig:"MIGRATION_PLANNER_LOG_LEVEL" default:"info"`
}

type KafkaConfig struct {
	Brokers  []string            `envconfig:"MIGRATION_PLANNER_KAFKA_BROKERS" default:""`
	Topic    string              `envconfig:"MIGRATION_PLANNER_KAFKA_TOPIC" default:""`
	Version  sarama.KafkaVersion `envconfig:"MIGRATION_PLANNER_KAFKA_VERSION" default:""`
	ClientID string              `envconfig:"MIGRATION_PLANNER_KAFKA_CLIENT_ID" default:""`

	SaramaConfig *sarama.Config
}

func New() (*Config, error) {
	if singleConfig == nil {
		singleConfig = new(Config)
		if err := envconfig.Process(EnvConfigPrefix, singleConfig); err != nil {
			return nil, err
		}
	}
	return singleConfig, nil
}
