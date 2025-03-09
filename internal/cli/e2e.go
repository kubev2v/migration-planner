package cli

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

type E2ETestOptions struct {
	clusterName               string
	plannerAPIImage           string
	plannerAPIImagePullPolicy string
	containerRuntime          string
	localRegistryPort         string
	pkgManager                string
	insecureRegistryAddr      string
	agentImage                string
	registryIP                net.IP
	destroyEnvironment        bool
	iface                     string
}

const (
	defaultClusterName        = "kind-e2e"
	defaultPlannerAPIImage    = "custom/migration-planner-api"
	defaultPullPolicy         = "Never"
	defaultContainerRuntime   = "docker"
	defaultRegistryPort       = "5000"
	defaultPlannerServicePort = "3443"
	defaultDestroyEnv         = true
)

func DefaultE2EOptions() *E2ETestOptions {
	defaultRegistryIP, _ := getLocalIP()
	defaultInsecureRegistry := fmt.Sprintf("%s:%s", defaultRegistryIP, defaultRegistryPort)
	defaultPlannerAgentImage := fmt.Sprintf("%s/agent", defaultInsecureRegistry)
	defaultNetworkInterface := getInterfaceName(defaultRegistryIP)

	return &E2ETestOptions{
		clusterName:               defaultClusterName,
		plannerAPIImage:           defaultPlannerAPIImage,
		plannerAPIImagePullPolicy: defaultPullPolicy,
		containerRuntime:          defaultContainerRuntime,
		localRegistryPort:         defaultRegistryPort,
		insecureRegistryAddr:      defaultInsecureRegistry,
		agentImage:                defaultPlannerAgentImage,
		pkgManager:                getPackageManager(),
		registryIP:                defaultRegistryIP,
		destroyEnvironment:        defaultDestroyEnv,
		iface:                     defaultNetworkInterface,
	}
}

func NewCmdE2E() *cobra.Command {
	o := DefaultE2EOptions()
	cmd := &cobra.Command{
		Use:   "e2e",
		Short: "Running the e2e test locally",
		Example: "e2e -d=false\n" +
			"This command creates the environment, runs the e2e test and prevents the deletion of the Kind cluster.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return o.Run(cmd.Context(), args)
		},
		SilenceUsage: true,
	}
	o.Bind(cmd.Flags())
	return cmd
}

func (o *E2ETestOptions) Bind(fs *pflag.FlagSet) {
	fs.BoolVarP(&o.destroyEnvironment, "destroy-env", "d", defaultDestroyEnv, fmt.Sprintf("Destroy the created %s cluster", o.clusterName))
}

func (o *E2ETestOptions) Run(ctx context.Context, args []string) error {
	if err := o.configureEnvironment(); err != nil {
		return err
	}

	isDeployEnvironmentRequired := true
	if kindClusterExists(ctx, o.clusterName) {
		log.Printf("Cluster %s already exists, proceeding...", o.clusterName)
		isDeployEnvironmentRequired = false
	}
	if isDeployEnvironmentRequired {
		if err := o.deployEnvironment(ctx); err != nil {
			log.Fatalf("Failed to deploy environment. Error: %v", err)
		}
	}

	if err := o.waitForMigrationPlannerService(ctx); err != nil {
		_ = destroyEnvironment(ctx)
		return err
	}

	o.executeTest(ctx)

	if o.destroyEnvironment {
		if err := destroyEnvironment(ctx); err != nil {
			log.Fatalf("Failed to destroy environment. Error: %v", err)
		}
	}

	return nil
}

func getPackageManager() string {
	if _, err := exec.LookPath("dnf"); err == nil {
		return "dnf"
	} else if _, err := exec.LookPath("apt"); err == nil {
		return "apt"
	}
	return ""
}

func runCommand(ctx context.Context, cmdStr string) error {
	cmd := exec.CommandContext(ctx, "bash", "-c", cmdStr)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (o *E2ETestOptions) configureEnvironment() error {
	envVars := map[string]string{
		"REGISTRY_IP":                             o.registryIP.String(),
		"INSECURE_REGISTRY":                       o.insecureRegistryAddr,
		"MIGRATION_PLANNER_AGENT_IMAGE":           o.agentImage,
		"MIGRATION_PLANNER_API_IMAGE":             o.plannerAPIImage,
		"MIGRATION_PLANNER_API_IMAGE_PULL_POLICY": o.plannerAPIImagePullPolicy,
		"PODMAN":      o.containerRuntime,
		"PKG_MANAGER": o.pkgManager,
		"IFACE":       o.iface,
	}

	for key, value := range envVars {
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("failed to set env variable %s: %w", key, err)
		}
	}

	o.printConfig(envVars)

	return nil
}

func (o *E2ETestOptions) printConfig(envVars map[string]string) {
	// Print the Environment Variables Configuration
	fmt.Printf("====================================\n")
	fmt.Printf("🔧 Environment Variables Configured:\n")
	fmt.Printf("====================================\n")
	for key, value := range envVars {
		fmt.Printf("%s=%s\n", key, value)
	}
	fmt.Printf("====================================\n")

	// Print the Test Execution Configuration
	fmt.Printf("====================================\n")
	fmt.Printf("🛠 Test Execution Configuration:\n")
	fmt.Printf("====================================\n")
	fmt.Printf("Destroy environment at the end? %v\n", o.destroyEnvironment)
	fmt.Printf("====================================\n")
}

func kindClusterExists(ctx context.Context, clusterName string) bool {
	cmd := exec.CommandContext(ctx, "kind", "get", "clusters")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error checking Kind clusters: %v\n", err)
		return false
	}

	return strings.Contains(string(output), clusterName)
}

func (o *E2ETestOptions) deployEnvironment(ctx context.Context) error {
	command := "make deploy-e2e-environment"
	return runCommand(ctx, command)
}

func destroyEnvironment(ctx context.Context) error {
	command := "make undeploy-e2e-environment"
	return runCommand(ctx, command)
}

func (o *E2ETestOptions) executeTest(ctx context.Context) {
	command := fmt.Sprintf("make integration-test PLANNER_IP=%s", o.registryIP)
	if err := runCommand(ctx, command); err != nil {
		fmt.Printf("Failed to execute: %s, Error: %v\n", command, err)
	}
}

func getInterfaceName(ip net.IP) string {

	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	for _, iface := range interfaces {
		address, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range address {
			if ipNet, ok := addr.(*net.IPNet); ok && ipNet.IP.Equal(ip) {
				return iface.Name
			}
		}
	}

	return ""
}

func waitForService(ctx context.Context, host string, port string, fixCommand string) error {
	address := fmt.Sprintf("%s:%s", host, port)
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	channel := make(chan error)

	go func() {
		defer close(channel)
		for {
			select {
			case <-ctxWithTimeout.Done():
				channel <- fmt.Errorf("timeout waiting for %s", address)
				return
			case <-time.After(2 * time.Second):
				fmt.Printf("Waiting for address: %s to become available\n", address)

				conn, err := net.DialTimeout("tcp", address, 2*time.Second)
				if err == nil {
					fmt.Printf("Address: %s is available\n", address)
					_ = conn.Close()
					channel <- nil
					return
				}

				_ = runCommand(ctxWithTimeout, fixCommand)
			}
		}
	}()

	return <-channel
}

func (o *E2ETestOptions) waitForMigrationPlannerService(ctx context.Context) error {
	portForwardCommand := fmt.Sprintf("kubectl port-forward --address 0.0.0.0 service/migration-planner %s:%s &",
		defaultPlannerServicePort, defaultPlannerServicePort)
	return waitForService(ctx, o.registryIP.String(), defaultPlannerServicePort, portForwardCommand)
}
