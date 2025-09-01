package agent

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/kubev2v/migration-planner/test/e2e/service"
	. "github.com/kubev2v/migration-planner/test/e2e/utils"
	libvirt "github.com/libvirt/libvirt-go"
	. "github.com/onsi/ginkgo/v2"
	"go.uber.org/zap"
)

// PlannerAgent defines the interface for interacting with a planner agent instance
type PlannerAgent interface {
	DumpLogs(string)
	GetIp() (string, error)
	IsServiceRunning(string, string) bool
	Run() error
	Restart() error
	Remove() error
}

// plannerAgentLibvirt is an implementation of the PlannerAgent interface
type plannerAgentLibvirt struct {
	agentEndToEndTestID string
	con                 *libvirt.Connect
	name                string
	service             PlannerService
	sourceID            uuid.UUID
}

// NewPlannerAgent creates a new libvirt-based planner agent instance
func NewPlannerAgent(sourceID uuid.UUID, name string, idForTest string, svc PlannerService) (*plannerAgentLibvirt, error) {

	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
		return nil, fmt.Errorf("failed to connect to hypervisor: %v", err)
	}
	return &plannerAgentLibvirt{agentEndToEndTestID: idForTest, con: conn,
		name: name, service: svc, sourceID: sourceID}, nil
}

// DumpLogs prints journal logs from the agent VM to the GinkgoWriter
func (p *plannerAgentLibvirt) DumpLogs(agentIp string) {
	stdout, _ := RunSSHCommand(agentIp, "journalctl --no-pager")
	fmt.Fprintf(GinkgoWriter, "Journal: %v\n", stdout)
}

// GetIp retrieves the IP address of the agent VM
func (p *plannerAgentLibvirt) GetIp() (string, error) {
	zap.S().Info("Attempting to retrieve agent IP")
	domain, err := p.con.LookupDomainByName(p.name)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = domain.Free()
	}()

	// Get VM IP:
	ifaceAddresses, err := domain.ListAllInterfaceAddresses(libvirt.DOMAIN_INTERFACE_ADDRESSES_SRC_LEASE)
	if err != nil {
		return "", err
	}

	for _, iface := range ifaceAddresses {
		for _, addr := range iface.Addrs {
			if addr.Type == libvirt.IP_ADDR_TYPE_IPV4 {
				return addr.Addr, nil
			}
		}
	}
	return "", fmt.Errorf("No IP found")
}

// IsServiceRunning checks whether the specified systemd service is running on the agent
func (p *plannerAgentLibvirt) IsServiceRunning(agentIp string, service string) bool {
	_, err := RunSSHCommand(agentIp, fmt.Sprintf("systemctl --user is-active --quiet %s", service))
	return err == nil
}

// Run prepares and launches the planner agent VM
func (p *plannerAgentLibvirt) Run() error {
	if err := p.prepareImage(); err != nil {
		return err
	}

	err := p.createVm()
	if err != nil {
		return err
	}

	return nil
}

// Restart reboots the agent VM gracefully and waits for it to return to a running state
func (p *plannerAgentLibvirt) Restart() error {
	domain, err := p.con.LookupDomainByName(p.name)
	if err != nil {
		return fmt.Errorf("failed to find vm: %w", err)
	}

	defer func() {
		_ = domain.Free()
	}()

	// power off the vm
	if err = domain.Shutdown(); err != nil {
		return fmt.Errorf("failed to shutdown vm: %w", err)
	}

	// Wait for shutdown with timeout
	if err = WaitForDomainState(30*time.Second, domain, libvirt.DOMAIN_SHUTOFF); err != nil {
		return fmt.Errorf("failed to reach shutdown state: %w", err)
	}

	// start the vm
	err = domain.Create()
	if err != nil {
		return fmt.Errorf("failed to start vm: %w", err)
	}

	// Wait for startup with timeout
	if err = WaitForDomainState(30*time.Second, domain, libvirt.DOMAIN_RUNNING); err != nil {
		return fmt.Errorf("failed to reach running state: %w", err)
	}

	return nil
}

// Remove gracefully deletes the virtual machine and associated resources
func (p *plannerAgentLibvirt) Remove() error {
	defer p.con.Close()
	domain, err := p.con.LookupDomainByName(p.name)
	if err != nil {
		return fmt.Errorf("failed to find domain: %v", err)
	}
	defer func() {
		_ = domain.Free()
	}()

	if state, _, err := domain.GetState(); err == nil && state == libvirt.DOMAIN_RUNNING {
		if err := domain.Destroy(); err != nil {
			return fmt.Errorf("failed to destroy domain: %v", err)
		}
	}

	if err := domain.Undefine(); err != nil {
		return fmt.Errorf("failed to undefine domain: %v", err)
	}

	if err := p.cleanupAgentFiles(); err != nil {
		return fmt.Errorf("failed to delete agent files: %v", err)
	}

	return nil
}
