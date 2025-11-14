package config

import (
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
	Auth                 Auth
	MigrationFolder      string `envconfig:"MIGRATION_PLANNER_MIGRATIONS_FOLDER" default:""`
	OpaPoliciesFolder    string `envconfig:"MIGRATION_PLANNER_OPA_POLICIES_FOLDER" default:"/app/policies"`
	S3                   S3
	IsoPath              string `envconfig:"MIGRATION_PLANNER_ISO_PATH" default:"rhcos-live-iso.x86_64.iso"`
	Authz                Authz
}

type Auth struct {
	AuthenticationType         string `envconfig:"MIGRATION_PLANNER_AUTH" default:""`
	JwkCertURL                 string `envconfig:"MIGRATION_PLANNER_JWK_URL" default:""`
	LocalPrivateKey            string `envconfig:"MIGRATION_PLANNER_PRIVATE_KEY" default:""`
	AgentAuthenticationEnabled bool   `envconfig:"MIGRATION_PLANNER_AGENT_AUTH_ENABLED" default:"true"`
}

type Authz struct {
	AuthorizationKind      string `envconfig:"MIGRATION_PLANNER_AUTHZ_KIND" default:"none"` // possible value: none, legacy, spicedb
	PlatformDefinitionFile string `envconfig:"MIGRATION_PLANNER_AUTHZ_PLATFORM_FILE" default:""`
	SpiceDBURL             string `envconfig:"MIGRATION_PLANNER_AUTHZ_SPICEDB_URL" default:"localhost:50051"`
	SpiceDBToken           string `envconfig:"MIGRATION_PLANNER_AUTHZ_SPICEDB_TOKEN" default:""`
}

type S3 struct {
	Endpoint    string `envconfig:"MIGRATION_PLANNER_S3_ENDPOINT" default:""`
	Bucket      string `envconfig:"MIGRATION_PLANNER_S3_BUCKET" default:""`
	AccessKey   string `envconfig:"MIGRATION_PLANNER_S3_ACCESS_KEY" default:""`
	SecretKey   string `envconfig:"MIGRATION_PLANNER_S3_SECRET_KEY" default:""`
	IsoFileName string `envconfig:"MIGRATION_PLANNER_S3_ISO_FILENAME" default:""`
	IsoSha256   string `envconfig:"MIGRATION_PLANNER_S3_ISO_SHA256" default:""`
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
