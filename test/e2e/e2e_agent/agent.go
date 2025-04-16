package e2e_agent

import (
	"fmt"
	"github.com/google/uuid"
	. "github.com/kubev2v/migration-planner/test/e2e/e2e_service"
	. "github.com/kubev2v/migration-planner/test/e2e/e2e_utils"
	libvirt "github.com/libvirt/libvirt-go"
	. "github.com/onsi/ginkgo/v2"
	"go.uber.org/zap"
	"time"
)

type PlannerAgent interface {
	AgentApi() *AgentApi
	DumpLogs(string)
	GetIp() (string, error)
	IsServiceRunning(string, string) bool
	Run() error
	Restart() error
	Remove() error
	SetAgentApi(*AgentApi)
}

type plannerAgentLibvirt struct {
	agentEndToEndTestID string
	con                 *libvirt.Connect
	localApi            *AgentApi
	name                string
	serviceApi          *ServiceApi
	sourceID            uuid.UUID
}

func NewPlannerAgent(sourceID uuid.UUID, name string, idForTest string) (*plannerAgentLibvirt, error) {

	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
		return nil, fmt.Errorf("failed to connect to hypervisor: %v", err)
	}
	return &plannerAgentLibvirt{agentEndToEndTestID: idForTest, con: conn,
		name: name, serviceApi: NewServiceApi(), sourceID: sourceID}, nil
}

func (p *plannerAgentLibvirt) AgentApi() *AgentApi {
	return p.localApi
}

func (p *plannerAgentLibvirt) DumpLogs(agentIp string) {
	stdout, _ := RunSSHCommand(agentIp, "journalctl --no-pager")
	fmt.Fprintf(GinkgoWriter, "Journal: %v\n", stdout)
}

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

func (p *plannerAgentLibvirt) IsServiceRunning(agentIp string, service string) bool {
	_, err := RunSSHCommand(agentIp, fmt.Sprintf("systemctl --user is-active --quiet %s", service))
	return err == nil
}

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

func (p *plannerAgentLibvirt) SetAgentApi(agentApi *AgentApi) {
	p.localApi = agentApi
}
