package e2e_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	internalclient "github.com/kubev2v/migration-planner/internal/api/client"
	"github.com/kubev2v/migration-planner/internal/auth"
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
	defaultBasePath   string = "/tmp/untarova/"
	defaultIsoPath           = filepath.Join(defaultBasePath, "agent.iso")
	defaultVmdkName          = filepath.Join(defaultBasePath, "persistence-disk.vmdk")
	defaultOvaPath    string = filepath.Join(home, "myimage.ova")
	defaultServiceUrl string = fmt.Sprintf("http://%s:3443", os.Getenv("PLANNER_IP"))
)

type PlannerAgent interface {
	Run() error
	Login(url string, user string, pass string) (*http.Response, error)
	Version() (string, error)
	Restart() error
	Remove() error
	GetIp() (string, error)
	IsServiceRunning(string, string) bool
	DumpLogs(string)
}

type PlannerService interface {
	RemoveSources() error
	GetSource(id uuid.UUID) (*api.Source, error)
	CreateSource(name string) (*api.Source, error)
}

type plannerService struct {
	c *internalclient.ClientWithResponses
}

type plannerAgentLibvirt struct {
	c        *internalclient.ClientWithResponses
	name     string
	con      *libvirt.Connect
	sourceID uuid.UUID
}

func NewPlannerAgent(configPath string, sourceID uuid.UUID, name string) (*plannerAgentLibvirt, error) {
	_ = createConfigFile(configPath)

	c, err := client.NewFromConfigFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}

	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
		return nil, fmt.Errorf("failed to connect to hypervisor: %v", err)
	}

	return &plannerAgentLibvirt{c: c, name: name, con: conn, sourceID: sourceID}, nil
}

func (p *plannerAgentLibvirt) Run() error {
	if err := p.prepareImage(p.sourceID); err != nil {
		return err
	}

	err := CreateVm(p.con)
	if err != nil {
		return err
	}

	return nil
}

func (p *plannerAgentLibvirt) prepareImage(sourceID uuid.UUID) error {
	// Create OVA:
	ovaFile, err := os.Create(defaultOvaPath)
	if err != nil {
		return err
	}
	defer os.Remove(ovaFile.Name())

	if err = os.Mkdir(defaultBasePath, os.ModePerm); err != nil {
		if !os.IsExist(err) {
			return fmt.Errorf("creating default base path: %w", err)
		}
	}

	user := auth.User{
		Username:     "admin",
		Organization: "admin",
	}
	ctx := auth.NewUserContext(context.TODO(), user)

	// Download OVA

	res, err := p.c.GetImage(ctx, sourceID)
	if err != nil {
		return fmt.Errorf("error getting source image: %w", err)
	}
	defer res.Body.Close()

	if _, err = io.Copy(ovaFile, res.Body); err != nil {
		return fmt.Errorf("error writing to file: %w", err)
	}

	if err = ValidateTar(ovaFile); err != nil {
		return fmt.Errorf("error validating tar: %w", err)
	}

	// Untar ISO from OVA
	if err = Untar(ovaFile, defaultIsoPath, "MigrationAssessment.iso"); err != nil {
		return fmt.Errorf("error uncompressing the file: %w", err)
	}

	// Untar VMDK from OVA
	if err = Untar(ovaFile, defaultVmdkName, "persistence-disk.vmdk"); err != nil {
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
	req, err := http.NewRequest("GET", fmt.Sprintf("https://%s:3333/api/v1/version", agentIP), nil)
	if err != nil {
		return "", fmt.Errorf("Failed to create request: %v", err)
	}

	// Set the Content-Type header to application/json
	req.Header.Set("Content-Type", "application/json")

	// Send the request using http.DefaultClient
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
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
		fmt.Sprintf("https://%s:3333/api/v1/credentials", agentIP),
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	resp.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
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
	if err = waitForDomainState(30*time.Second, domain, libvirt.DOMAIN_SHUTOFF); err != nil {
		return fmt.Errorf("failed to reach shutdown state: %w", err)
	}

	// start the vm
	err = domain.Create()
	if err != nil {
		return fmt.Errorf("failed to start vm: %w", err)
	}

	// Wait for startup with timeout
	if err = waitForDomainState(30*time.Second, domain, libvirt.DOMAIN_RUNNING); err != nil {
		return fmt.Errorf("failed to reach running state: %w", err)
	}

	return nil
}

func waitForDomainState(duration time.Duration, domain *libvirt.Domain, desiredState libvirt.DomainState) error {
	timeout := time.After(duration)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timed out waiting for desired state")
		case <-ticker.C:
			state, _, err := domain.GetState()
			if err != nil {
				return fmt.Errorf("failed to get VM state: %w", err)
			}
			if state == desiredState {
				return nil
			}
		}
	}
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

func (s *plannerService) CreateSource(name string) (*api.Source, error) {
	user := auth.User{
		Username:     "admin",
		Organization: "admin",
	}
	ctx := auth.NewUserContext(context.TODO(), user)

	params := v1alpha1.CreateSourceJSONRequestBody{Name: name}
	res, err := s.c.CreateSourceWithResponse(ctx, params)
	if err != nil || res.HTTPResponse.StatusCode != 201 {
		return nil, fmt.Errorf("Error creating the source: %s", err)
	}

	if res.JSON201 == nil {
		return nil, fmt.Errorf("Error creating the source")
	}

	return res.JSON201, nil
}

func (s *plannerService) GetSource(id uuid.UUID) (*api.Source, error) {
	user := auth.User{
		Username:     "admin",
		Organization: "admin",
	}
	ctx := auth.NewUserContext(context.TODO(), user)

	res, err := s.c.GetSourceWithResponse(ctx, id)
	if err != nil || res.HTTPResponse.StatusCode != 200 {
		return nil, fmt.Errorf("Error listing sources")
	}

	return res.JSON200, nil
}

func (s *plannerService) RemoveSources() error {
	user := auth.User{
		Username:     "admin",
		Organization: "admin",
	}
	ctx := auth.NewUserContext(context.TODO(), user)

	_, err := s.c.DeleteSourcesWithResponse(ctx)
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
