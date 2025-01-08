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
	Run() error
	Login(url string, user string, pass string) (*http.Response, error)
	Version() (string, error)
	Remove() error
	GetIp() (string, error)
	IsServiceRunning(string, string) bool
	DumpLogs(string)
}

type PlannerService interface {
	RemoveSources() error
	RemoveAgent(UUID string) error
	GetSource() (*api.Source, error)
	GetAgent(credentialUrl string) (*api.Agent, error)
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

func (p *plannerAgentLibvirt) Run() error {
	if err := p.prepareImage(); err != nil {
		return err
	}

	err := CreateVm(p.con)
	if err != nil {
		return err
	}

	return nil
}

func (p *plannerAgentLibvirt) prepareImage() error {
	// Create OVA:
	file, err := os.Create(defaultOvaPath)
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())

	// Download OVA
	res, err := p.c.GetImage(context.TODO(), &api.GetImageParams{})
	if err != nil {
		return fmt.Errorf("error getting source image: %w", err)
	}
	defer res.Body.Close()

	if _, err = io.Copy(file, res.Body); err != nil {
		return fmt.Errorf("error writing to file: %w", err)
	}

	// Untar ISO from OVA
	if err = Untar(file, defaultIsoPath, "MigrationAssessment.iso"); err != nil {
		return fmt.Errorf("error uncompressing the file: %w", err)
	}

	return nil
}

func (p *plannerAgentLibvirt) Version() (string, error) {
	agentIP, err := p.GetIp()
	if err != nil {
		return "", fmt.Errorf("failed to get agent IP: %w", err)
	}
	// Create a new HTTP GET request
	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:3333/api/v1/version", agentIP), nil)
	if err != nil {
		return "", fmt.Errorf("Failed to create request: %v", err)
	}

	// Set the Content-Type header to application/json
	req.Header.Set("Content-Type", "application/json")

	// Send the request using http.DefaultClient
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Read the response body
	var result struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %v", err)
	}
	return result.Version, nil
}

func (p *plannerAgentLibvirt) Login(url string, user string, pass string) (*http.Response, error) {
	agentIP, err := p.GetIp()
	if err != nil {
		return nil, fmt.Errorf("failed to get agent IP: %w", err)
	}

	credentials := map[string]string{
		"url":      url,
		"username": user,
		"password": pass,
	}

	jsonData, err := json.Marshal(credentials)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal credentials: %w", err)
	}

	resp, err := http.NewRequest(
		"PUT",
		fmt.Sprintf("http://%s:3333/api/v1/credentials", agentIP),
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	resp.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	response, err := client.Do(resp)
	if err != nil {
		return response, fmt.Errorf("failed to send request: %w", err)
	}
	defer response.Body.Close()

	return response, nil
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
	stdout, _ := RunCommand(agentIp, "journalctl --no-pager")
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

func (s *plannerService) GetAgent(credentialUrl string) (*api.Agent, error) {
	ctx := context.TODO()
	res, err := s.c.ListAgentsWithResponse(ctx)
	if err != nil || res.HTTPResponse.StatusCode != 200 {
		return nil, fmt.Errorf("Error listing agents")
	}

	if len(*res.JSON200) == 1 {
		return nil, fmt.Errorf("No agents found")
	}

	for _, agent := range *res.JSON200 {
		if agent.CredentialUrl == credentialUrl {
			return &agent, nil
		}
	}

	return nil, fmt.Errorf("No agents found")
}

func (s *plannerService) GetSource() (*api.Source, error) {
	ctx := context.TODO()
	res, err := s.c.ListSourcesWithResponse(ctx)
	if err != nil || res.HTTPResponse.StatusCode != 200 {
		return nil, fmt.Errorf("Error listing sources")
	}

	if len(*res.JSON200) == 1 {
		return nil, fmt.Errorf("No sources found")
	}

	nullUuid := uuid.UUID{}
	for _, source := range *res.JSON200 {
		if source.Id != nullUuid {
			return &source, nil
		}
	}

	return nil, fmt.Errorf("No sources found")
}

func (s *plannerService) RemoveAgent(UUID string) error {
	parsedUUID, err1 := uuid.Parse(UUID)
	if err1 != nil {
		return err1
	}
	_, err2 := s.c.DeleteAgentWithResponse(context.TODO(), parsedUUID)
	if err2 != nil {
		return err2
	}
	return nil
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
