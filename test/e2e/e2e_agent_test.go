package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	internalclient "github.com/kubev2v/migration-planner/internal/api/client"
	"github.com/kubev2v/migration-planner/internal/client"
	libvirt "github.com/libvirt/libvirt-go"
	. "github.com/onsi/ginkgo/v2"
)

const (
	vmName string = "coreos-vm"
)

var (
	home              string = os.Getenv("HOME")
	defaultConfigPath string = filepath.Join(home, ".config/planner/client.yaml")
	defaultIsoPath    string = "/tmp/agent.iso"
	defaultOvaPath    string = filepath.Join(home, "myimage.ova")
	defaultServiceUrl string = fmt.Sprintf("http://%s:3443", os.Getenv("PLANNER_IP"))
)

type PlannerAgent interface {
	Run(string) error
	Login(url string, user string, pass string) error
	Remove() error
	GetIp() (string, error)
	IsServiceRunning(string, string) bool
	DumpLogs(string)
}

type PlannerService interface {
	Create(name string) (string, error)
	RemoveSources() error
	GetSource() (*api.Source, error)
}

type plannerService struct {
	c *internalclient.ClientWithResponses
}

type plannerAgentLibvirt struct {
	c    *internalclient.ClientWithResponses
	name string
	con  *libvirt.Connect
}

func NewPlannerAgent(configPath string, name string) (*plannerAgentLibvirt, error) {
	_ = createConfigFile(configPath)

	c, err := client.NewFromConfigFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}

	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
		return nil, fmt.Errorf("failed to connect to hypervisor: %v", err)
	}

	return &plannerAgentLibvirt{c: c, name: name, con: conn}, nil
}

func (p *plannerAgentLibvirt) Run(sourceId string) error {
	if err := p.prepareImage(sourceId); err != nil {
		return err
	}

	err := CreateVm(p.con)
	if err != nil {
		return err
	}

	return nil
}

func (p *plannerAgentLibvirt) prepareImage(sourceId string) error {
	// Create OVA:
	file, err := os.Create(defaultOvaPath)
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())

	// Download OVA
	res, err := p.c.GetSourceImage(context.TODO(), uuid.MustParse(sourceId))
	if err != nil {
		return fmt.Errorf("error getting source image: %w", err)
	}
	defer res.Body.Close()

	if _, err = io.Copy(file, res.Body); err != nil {
		return fmt.Errorf("error writing to file: %w", err)
	}

	// Untar ISO from OVA
	if err = Untar(file, defaultIsoPath, "AgentVM-1.iso"); err != nil {
		return fmt.Errorf("error uncompressing the file: %w", err)
	}

	return nil
}

func (p *plannerAgentLibvirt) Login(url string, user string, pass string) error {
	agentIP, err := p.GetIp()
	if err != nil {
		return fmt.Errorf("failed to get agent IP: %w", err)
	}

	credentials := map[string]string{
		"url":      url,
		"username": user,
		"password": pass,
	}

	jsonData, err := json.Marshal(credentials)
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	resp, err := http.NewRequest(
		"PUT",
		fmt.Sprintf("http://%s:3333/api/v1/credentials", agentIP),
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	resp.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	response, err := client.Do(resp)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer response.Body.Close()

	return nil
}

func (p *plannerAgentLibvirt) RestartService() error {
	return nil
}

func (p *plannerAgentLibvirt) Remove() error {
	defer p.con.Close()
	domain, err := p.con.LookupDomainByName(p.name)
	if err != nil {
		return err
	}
	defer func() {
		_ = domain.Free()
	}()

	if state, _, err := domain.GetState(); err == nil && state == libvirt.DOMAIN_RUNNING {
		if err := domain.Destroy(); err != nil {
			return err
		}
	}

	if err := domain.Undefine(); err != nil {
		return err
	}

	// Remove the ISO file if it exists
	if _, err := os.Stat(defaultIsoPath); err == nil {
		if err := os.Remove(defaultIsoPath); err != nil {
			return fmt.Errorf("failed to remove ISO file: %w", err)
		}
	}

	return nil
}

func (p *plannerAgentLibvirt) GetIp() (string, error) {
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
	_, err := RunCommand(agentIp, fmt.Sprintf("systemctl --user is-active --quiet %s", service))
	return err == nil
}

func (p *plannerAgentLibvirt) DumpLogs(agentIp string) {
	stdout, _ := RunCommand(agentIp, "journalctl --no-pager --user -u planner-agent")
	fmt.Fprintf(GinkgoWriter, "Journal: %v\n", stdout)
}

func NewPlannerService(configPath string) (*plannerService, error) {
	_ = createConfigFile(configPath)
	c, err := client.NewFromConfigFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}

	return &plannerService{c: c}, nil
}

func (s *plannerService) Create(name string) (string, error) {
	ctx := context.TODO()
	body := api.SourceCreate{Name: name}
	_, err := s.c.CreateSource(ctx, body)
	if err != nil {
		return "", fmt.Errorf("Error creating source")
	}

	source, err := s.GetSource()
	if err != nil {
		return "", err
	}

	return source.Id.String(), nil
}

func (s *plannerService) GetSource() (*api.Source, error) {
	ctx := context.TODO()
	res, err := s.c.ListSourcesWithResponse(ctx)
	if err != nil || res.HTTPResponse.StatusCode != 200 {
		return nil, fmt.Errorf("Error listing sources")
	}

	if len(*res.JSON200) == 0 {
		return nil, fmt.Errorf("No sources found")
	}

	return &(*res.JSON200)[0], nil
}

func (s *plannerService) RemoveSources() error {
	_, err := s.c.DeleteSourcesWithResponse(context.TODO())
	return err
}

func createConfigFile(configPath string) error {
	// Ensure the directory exists
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory structure: %w", err)
	}

	// Create configuration
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return os.WriteFile(configPath, []byte(fmt.Sprintf("service:\n  server: %s", defaultServiceUrl)), 0644)
	}

	return nil
}
