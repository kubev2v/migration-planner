package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type VcsimInstance struct {
	Name     string
	Port     int
	Username string
	Password string
}

func (v VcsimInstance) SdkURL(hostIP string) string {
	return fmt.Sprintf("https://%s:%d/sdk", hostIP, v.Port)
}

type InfraConfig struct {
	ClusterName          string
	APIImage             string
	ISOImage             string
	PrivateKeyPath       string
	HostIP               string
	Namespace            string
	RelativeTemplatesDir string
	KeepEnv              bool
	SkipSetup            bool

	// Service deployment
	ServiceAPIPath       string
	ImagePullPolicy      string
	PersistentDiskDevice string
	AuthMethod           string
	AdminGroupFile       string
	ServiceReplicas      string
	UIPort               int

	// Port-forwards
	PlannerAgentPort    int
	PlannerAgentService string

	// Vcsim instances
	Vcsim []VcsimInstance

	// Admin bootstrap
	AdminUsername string
	AdminEmail    string

	// RHCOS
	RHCOSPassword string

	// Postgres
	PostgresDeployName string
}

type TestConfig struct {
	DefaultOrganization string
	DefaultUsername     string
	DefaultEmailDomain  string
	DefaultBasePath     string
	Home                string
	TestsExecutionTime  map[string]time.Duration
}

type Config struct {
	Infra InfraConfig
	Test  TestConfig
}

var Cfg = Config{
	Infra: InfraConfig{
		Namespace:            "default",
		RelativeTemplatesDir: filepath.Join("deploy", "templates"),
		ServiceAPIPath:       "/api/migration-assessment",
		ImagePullPolicy:      "Never",
		PersistentDiskDevice: "/dev/vda",
		AuthMethod:           "local",
		AdminGroupFile:       "/etc/planner/admin-group.yaml",
		ServiceReplicas:      "1",
		UIPort:               3333,
		PlannerAgentPort:     7443,
		PlannerAgentService:  "service/migration-planner-agent",
		RHCOSPassword:        `$$y$$j9T$$hUUbW8zoB.Qcmpwm4/RuK1$$FMtuDAxNLp3sEa2PnGiJdXr8uYbvUNPlVDXpcJim529`,
		PostgresDeployName:   "migration-planner-postgres",
		AdminUsername:        "admin",
		AdminEmail:           "admin@example.com",
		Vcsim: []VcsimInstance{
			{Name: "vcsim1", Port: 8989, Username: "core", Password: "123456"},
			{Name: "vcsim2", Port: 8990, Username: "core", Password: "123456"},
		},
	},
	Test: TestConfig{
		DefaultOrganization: "admin",
		DefaultUsername:     "admin",
		DefaultEmailDomain:  "example.com",
		DefaultBasePath:     "/tmp/untarova/",
		Home:                os.Getenv("HOME"),
		TestsExecutionTime:  make(map[string]time.Duration),
	},
}

func (c Config) PrivateKeyFilePath() string {
	return c.Infra.PrivateKeyPath + "/private-key"
}

func (c Config) ServiceUrl() string {
	return fmt.Sprintf("http://%s:%d%s", c.Infra.HostIP, c.Infra.PlannerAgentPort, c.Infra.ServiceAPIPath)
}
