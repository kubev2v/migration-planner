package config

import (
	"github.com/IBM/sarama"
	"github.com/kelseyhightower/envconfig"
)

var singleConfig *Config = nil

type Config struct {
	Database *dbConfig
	Service  *svcConfig
}

type dbConfig struct {
	Type     string `envconfig:"DB_TYPE" default:"pgsql"`
	Hostname string `envconfig:"DB_HOST" default:"localhost"`
	Port     string `envconfig:"DB_PORT" default:"5432"`
	Name     string `envconfig:"DB_NAME" default:"planner"`
	User     string `envconfig:"DB_USER" default:"admin"`
	Password string `envconfig:"DB_PASS" default:"adminpass"`
}

type svcConfig struct {
	Address              string `envconfig:"MIGRATION_PLANNER_ADDRESS" default:":3443"`
	AgentEndpointAddress string `envconfig:"MIGRATION_PLANNER_AGENT_ENDPOINT_ADDRESS" default:":7443"`
	ImageEndpointAddress string `envconfig:"MIGRATION_PLANNER_IMAGE_ENDPOINT_ADDRESS" default:":11443"`
	BaseUrl              string `envconfig:"MIGRATION_PLANNER_BASE_URL" default:"https://localhost:3443"`
	BaseAgentEndpointUrl string `envconfig:"MIGRATION_PLANNER_BASE_AGENT_ENDPOINT_URL" default:"https://localhost:7443"`
	BaseImageEndpointUrl string `envconfig:"MIGRATION_PLANNER_BASE_IMAGE_ENDPOINT_URL" default:"https://localhost:11443"`
	LogLevel             string `envconfig:"MIGRATION_PLANNER_LOG_LEVEL" default:"info"`
	Kafka                kafkaConfig
	Auth                 Auth
	MigrationFolder      string `envconfig:"MIGRATION_PLANNER_MIGRATIONS_FOLDER" default:""`
}

type kafkaConfig struct {
	Brokers  []string            `envconfig:"MIGRATION_PLANNER_KAFKA_BROKERS" default:""`
	Topic    string              `envconfig:"MIGRATION_PLANNER_KAFKA_TOPIC" default:""`
	Version  sarama.KafkaVersion `envconfig:"MIGRATION_PLANNER_KAFKA_VERSION" default:""`
	ClientID string              `envconfig:"MIGRATION_PLANNER_KAFKA_CLIENT_ID" default:""`

	SaramaConfig *sarama.Config
}

type Auth struct {
	AuthenticationType string `envconfig:"MIGRATION_PLANNER_AUTH" default:""`
	JwkCertURL         string `envconfig:"MIGRATION_PLANNER_JWK_URL" default:""`
	LocalPrivateKey    string `envconfig:"MIGRATION_PLANNER_PRIVATE_KEY" default:""`
}

func New() (*Config, error) {
	if singleConfig == nil {
		singleConfig = new(Config)
		if err := envconfig.Process("", singleConfig); err != nil {
			return nil, err
		}
	}
	return singleConfig, nil
}
