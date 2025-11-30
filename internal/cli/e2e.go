package cli

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/kubev2v/migration-planner/test/e2e"
	"github.com/libvirt/libvirt-go"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
	isoImage                  string
	registryIP                net.IP
	keepEnvironment           bool
	iface                     string
}

const (
	defaultClusterName      = "kind-e2e"
	defaultPlannerAPIImage  = "custom/migration-planner-api"
	defaultPullPolicy       = "Never"
	defaultContainerRuntime = "docker"
	defaultRegistryPort     = "5000"
	defaultIsoImage         = "custom/migration-planner-iso"
)

func DefaultE2EOptions() *E2ETestOptions {
	defaultRegistryIP, _ := getLocalIP()
	defaultInsecureRegistry := fmt.Sprintf("%s:%s", defaultRegistryIP, defaultRegistryPort)

	return &E2ETestOptions{
		clusterName:               defaultClusterName,
		plannerAPIImage:           defaultPlannerAPIImage,
		plannerAPIImagePullPolicy: defaultPullPolicy,
		containerRuntime:          defaultContainerRuntime,
		localRegistryPort:         defaultRegistryPort,
		insecureRegistryAddr:      defaultInsecureRegistry,
		agentImage:                fmt.Sprintf("%s/agent", defaultInsecureRegistry),
		isoImage:                  defaultIsoImage,
		pkgManager:                getPackageManager(),
		registryIP:                defaultRegistryIP,
		iface:                     getInterfaceName(defaultRegistryIP),
	}
}

func NewCmdE2E() *cobra.Command {
	o := DefaultE2EOptions()
	cmd := &cobra.Command{
		Use:   "e2e",
		Short: "Running the e2e test locally",
		Example: "e2e -k\n" +
			"This command creates the environment, runs the e2e test and prevents the deletion of the Kind cluster.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return o.Run(cmd.Context())
		},
		SilenceUsage: true,
	}
	o.Bind(cmd.Flags())
	cmd.AddCommand(NewCmdE2EDestroy())
	return cmd
}

func NewCmdE2EDestroy() *cobra.Command {
	return &cobra.Command{
		Use:          "destroy",
		Short:        "Destroy the e2e test environment",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := DefaultE2EOptions().configureEnvironment(); err != nil {
				return err
			}
			return destroyEnvironment()
		},
	}
}

func (o *E2ETestOptions) Bind(fs *pflag.FlagSet) {
	fs.BoolVarP(&o.keepEnvironment, "keep-env", "k", false, fmt.Sprintf("Keep the created %s cluster", o.clusterName))
}

func (o *E2ETestOptions) Run(ctx context.Context) error {
	envVars, err := o.configureEnvironment()
	if err != nil {
		return err
	}
	o.printConfig(envVars)

	if shouldCreateCluster(ctx, o.clusterName) {
		if err := o.deployEnvironment(); err != nil {
			log.Fatalf("[CLI] Failed to deploy environment. Error: %v", err)
		}
	}

	log.Printf("[CLI] Cluster %s exists, proceeding...", o.clusterName)

	if err := validateVmsDeletion(); err != nil {
		log.Printf("[CLI] failed to delete old test VM's: %v", err)
		return err
	}

	o.executeTest()

	if o.keepEnvironment {
		return nil
	}

	if err := destroyEnvironment(); err != nil {
		log.Fatalf("[CLI] Failed to destroy environment. Error: %v", err)
	}

	return nil
}

// getPackageManager returns the name of the available system package manager (currently: dnf or apt)
func getPackageManager() string {
	if _, err := exec.LookPath("dnf"); err == nil {
		return "dnf"
	} else if _, err := exec.LookPath("apt"); err == nil {
		return "apt"
	}
	return ""
}

// runCommand executes a shell command in the context of the project root directory
func runCommand(cmdStr string) error {
	rootDir, err := findProjectRoot()
	if err != nil {
		return fmt.Errorf("[CLI] failed to find project root: %v", err)
	}
	cmd := exec.Command("bash", "-c", cmdStr)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = rootDir
	return cmd.Run()
}

// configureEnvironment sets up and exports environment variables needed for running the tests
func (o *E2ETestOptions) configureEnvironment() (map[string]string, error) {
	envVars := map[string]string{
		"REGISTRY_IP":                         o.registryIP.String(),
		"INSECURE_REGISTRY":                   o.insecureRegistryAddr,
		"MIGRATION_PLANNER_AGENT_IMAGE":       o.agentImage,
		"MIGRATION_PLANNER_API_IMAGE":         o.plannerAPIImage,
		"MIGRATION_PLANNER_IMAGE_PULL_POLICY": o.plannerAPIImagePullPolicy,
		"MIGRATION_PLANNER_ISO_IMAGE":         o.isoImage,
		"PODMAN":                              o.containerRuntime,
		"PKG_MANAGER":                         o.pkgManager,
		"IFACE":                               o.iface,
	}

	for key, value := range envVars {
		if err := os.Setenv(key, value); err != nil {
			return nil, fmt.Errorf("failed to set env variable %s: %w", key, err)
		}
	}

	return envVars, nil
}

// printConfig displays the configured environment variables and test settings
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
	fmt.Printf("Keep environment at the end? %v\n", o.keepEnvironment)
	fmt.Printf("====================================\n")
}

// shouldCreateCluster determines if a new Kind cluster should be created.
// It returns true if no running cluster with the given name exists
func shouldCreateCluster(ctx context.Context, clusterName string) bool {
	cmd := exec.CommandContext(ctx, "kind", "get", "clusters")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("[CLI] Error checking Kind clusters: %v\n", err)
		return true
	}

	if strings.Contains(string(output), clusterName) {
		return false
	}

	return true
}

// deployEnvironment triggers the Makefile target to deploy the full e2e test environment
func (o *E2ETestOptions) deployEnvironment() error {
	command := "make deploy-e2e-environment"
	return runCommand(command)
}

// destroyEnvironment tears down the e2e test environment using the Makefile target
func destroyEnvironment() error {
	log.Printf("[CLI] Destroying environment...")
	command := "make undeploy-e2e-environment"
	return runCommand(command)
}

// executeTest runs the integration tests using a Makefile target and the planner IP
func (o *E2ETestOptions) executeTest() {
	command := fmt.Sprintf("make integration-test PLANNER_IP=%s", o.registryIP)
	if err := runCommand(command); err != nil {
		fmt.Printf("Failed to execute: %s, Error: %v\n", command, err)
	}
}

// getInterfaceName attempts to find the network interface associated with the given IP
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

// validateVmsDeletion connects to libvirt and destroys and undefines any VMs created for the test
func validateVmsDeletion() error {
	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
		return fmt.Errorf("failed to connect to hypervisor: %v", err)
	}

	defer func() {
		_, _ = conn.Close()
	}()

	domains, err := conn.ListAllDomains(libvirt.CONNECT_LIST_DOMAINS_ACTIVE | libvirt.CONNECT_LIST_DOMAINS_INACTIVE)
	if err != nil {
		return fmt.Errorf("[CLI] Failed to list domains: %v", err)
	}

	for _, domain := range domains {
		name, err := domain.GetName()
		if err != nil {
			log.Printf("[CLI] Failed to get domain name: %v", err)
			_ = domain.Free()
			continue
		}

		if strings.HasPrefix(name, VmName) {
			if state, _, err := domain.GetState(); err == nil && state == libvirt.DOMAIN_RUNNING {
				if err := domain.Destroy(); err != nil {
					_ = domain.Free()
					return fmt.Errorf("failed to destroy domain: %v", err)
				}
			}

			if err := domain.Undefine(); err != nil {
				_ = domain.Free()
				return fmt.Errorf("failed to undefine domain: %v", err)
			}
		}

		_ = domain.Free()
	}

	return nil
}

// findProjectRoot searches for the project root by looking for a Makefile in the current or parent directory
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(dir, "Makefile")); err == nil {
		return dir, nil
	}

	parent := filepath.Dir(dir)
	if _, err := os.Stat(filepath.Join(parent, "Makefile")); err == nil {
		return parent, nil
	}

	return "", fmt.Errorf("error, Makefile not found in current or parent directory")
}
