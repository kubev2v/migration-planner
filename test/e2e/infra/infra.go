package infra

// InfraManager abstracts the e2e test infrastructure lifecycle.
// Kind-based: creates a Kind cluster and deploys workloads via client-go.
type InfraManager interface {
	CreateCluster() error
	DeleteCluster() error
	LoadImages(images []string) error

	DeploySecrets(privateKeyPath string) error
	DeployPostgres() error
	DeployVcsim() error
	DeployService(params map[string]string) error

	SetupPortForwards() error
	StopPortForwards() error
}
