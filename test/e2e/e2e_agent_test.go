package e2e_test

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	libvirt "github.com/libvirt/libvirt-go"
	. "github.com/onsi/ginkgo/v2"

	coreAgent "github.com/kubev2v/migration-planner/internal/agent"
)

var testOptions = struct {
	downloadImageByUrl      bool
	disconnectedEnvironment bool
}{}

const (
	vmName              string = "coreos-vm"
	defaultUsername     string = "admin"
	defaultOrganization string = "admin"
)

var (
	home               string = os.Getenv("HOME")
	defaultBasePath    string = "/tmp/untarova/"
	defaultVmdkName    string = filepath.Join(defaultBasePath, "persistence-disk.vmdk")
	defaultOvaPath     string = filepath.Join(home, "myimage.ova")
	defaultAgentTestID string = "1"
	privateKeyPath     string = filepath.Join(os.Getenv("E2E_PRIVATE_KEY_FOLDER_PATH"), "private-key")
)

type PlannerAgent interface {
	AgentApi() *AgentApi
	Run() error
	Restart() error
	Remove() error
	GetIp() (string, error)
	IsServiceRunning(string, string) bool
	DumpLogs(string)
}

type PlannerAgentAPI interface {
	Login(url string, user string, pass string) (*http.Response, error)
	Version() (string, error)
	Status() (*coreAgent.StatusReply, error)
	Inventory() (*v1alpha1.Inventory, error)
}

type plannerAgentLibvirt struct {
	serviceApi          *ServiceApi
	localApi            *AgentApi
	name                string
	con                 *libvirt.Connect
	sourceID            uuid.UUID
	agentEndToEndTestID string
}

type AgentApi struct {
	baseURL    string
	httpClient *http.Client
}

func NewPlannerAgent(sourceID uuid.UUID, name string, idForTest string) (*plannerAgentLibvirt, error) {

	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
		return nil, fmt.Errorf("failed to connect to hypervisor: %v", err)
	}

	return &plannerAgentLibvirt{serviceApi: NewServiceApi(), localApi: NewAgentApi(), name: name,
		con: conn, sourceID: sourceID, agentEndToEndTestID: idForTest}, nil
}

func (p *plannerAgentLibvirt) Run() error {
	if err := p.prepareImage(); err != nil {
		return err
	}

	err := p.CreateVm()
	if err != nil {
		return err
	}

	return nil
}

func (p *plannerAgentLibvirt) AgentApi() *AgentApi {
	return p.localApi
}

func (p *plannerAgentLibvirt) prepareImage() error {
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

	user, err := defaultUserAuth()
	if err != nil {
		return err
	}

	var res *http.Response

	if testOptions.downloadImageByUrl {
		url, err := p.getDownloadURL(user.Token.Raw)
		if err != nil {
			return err
		}

		res, err = http.Get(url) // Download OVA from the extracted URL
		if err != nil {
			return err
		}
	} else {
		getImagePath := p.sourceID.String() + "/" + "image"
		res, err = p.serviceApi.request(http.MethodGet, getImagePath, nil, user.Token.Raw)

		if err != nil {
			return fmt.Errorf("failed to get source image: %w", err)
		}
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download image: %s", res.Status)
	}

	if _, err = io.Copy(ovaFile, res.Body); err != nil {
		return fmt.Errorf("failed to write to the file: %w", err)
	}

	zap.S().Infof("Successfully downloaded ova file: %s", defaultOvaPath)

	if err := p.ovaValidateAndExtract(ovaFile); err != nil {
		return err
	}

	zap.S().Infof("Successfully extracted the Iso and Vmdk files from the OVA.")

	if err := ConvertVMDKtoQCOW2(defaultVmdkName, p.qcowDiskFilePath()); err != nil {
		return fmt.Errorf("failed to convert vmdk to qcow: %w", err)
	}

	zap.S().Infof("Successfully converted the vmdk to qcow.")

	return nil
}

func (p *plannerAgentLibvirt) getDownloadURL(jwtToken string) (string, error) {
	getImageUrlPath := p.sourceID.String() + "/" + "image-url"
	res, err := p.serviceApi.request(http.MethodGet, getImageUrlPath, nil, jwtToken)
	if err != nil {
		return "", fmt.Errorf("failed to get source url: %w", err)
	}
	defer res.Body.Close()

	var result struct {
		ExpiresAt string `json:"expires_at"`
		URL       string `json:"url"`
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to decod JSON: %w", err)
	}

	return result.URL, nil
}

func (p *plannerAgentLibvirt) ovaValidateAndExtract(ovaFile *os.File) error {
	if err := ValidateTar(ovaFile); err != nil {
		return fmt.Errorf("failed to validate tar: %w", err)
	}

	// Untar ISO from OVA
	if err := Untar(ovaFile, p.IsoFilePath(), "MigrationAssessment.iso"); err != nil {
		return fmt.Errorf("failed to uncompress the file: %w", err)
	}

	// Untar VMDK from OVA
	if err := Untar(ovaFile, defaultVmdkName, "persistence-disk.vmdk"); err != nil {
		return fmt.Errorf("failed to uncompress the file: %w", err)
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

	if err := RemoveFile(defaultOvaPath); err != nil {
		return fmt.Errorf("failed to remove OVA file: %w", err)
	}

	if err := RemoveFile(p.IsoFilePath()); err != nil {
		return fmt.Errorf("failed to remove ISO file: %w", err)
	}

	if err := RemoveFile(defaultVmdkName); err != nil {
		return fmt.Errorf("failed to remove Vmdk file: %w", err)
	}

	if err := RemoveFile(p.qcowDiskFilePath()); err != nil {
		return fmt.Errorf("failed to remove qcow disk file: %w", err)
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
	_, err := RunSSHCommand(agentIp, fmt.Sprintf("systemctl --user is-active --quiet %s", service))
	return err == nil
}

func (p *plannerAgentLibvirt) DumpLogs(agentIp string) {
	stdout, _ := RunSSHCommand(agentIp, "journalctl --no-pager")
	fmt.Fprintf(GinkgoWriter, "Journal: %v\n", stdout)
}

func (p *plannerAgentLibvirt) RestartService() error {
	return nil
}

func NewAgentApi() *AgentApi {
	return &AgentApi{
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		},
	}
}

func (api *AgentApi) request(method string, path string, body []byte, result any) (*http.Response, error) {
	var req *http.Request
	var err error

	queryPath := api.baseURL + path

	switch method {
	case http.MethodGet:
		req, err = http.NewRequest(http.MethodGet, queryPath, nil)
	case http.MethodPut:
		req, err = http.NewRequest(http.MethodPut, queryPath, bytes.NewReader(body))
	default:
		return nil, fmt.Errorf("unsupported method: %s", method)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	zap.S().Infof("[Agent-API] %s [Method: %s]", req.URL.String(), req.Method)
	res, err := api.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error getting response from local server: %v", err)
	}
	defer res.Body.Close()

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %v", err)
	}

	if result != nil {
		if err := json.Unmarshal(resBody, &result); err != nil {
			return nil, fmt.Errorf("error decoding JSON: %v", err)
		}
	}

	return res, nil
}

func (api *AgentApi) Version() (string, error) {
	var result struct {
		Version string `json:"version"`
	}

	res, err := api.request(http.MethodGet, "version", nil, &result)
	if err != nil || res.StatusCode != http.StatusOK {
		return "", err
	}
	return result.Version, nil
}

func (api *AgentApi) Login(url string, user string, pass string) (*http.Response, error) {
	zap.S().Infof("Attempting vCenter login with URL: %s, User: %s", url, user)

	credentials := map[string]string{
		"url":      url,
		"username": user,
		"password": pass,
	}

	jsonData, err := json.Marshal(credentials)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal credentials: %w", err)
	}

	res, err := api.request(http.MethodPut, "credentials", jsonData, nil)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (api *AgentApi) Status() (*coreAgent.StatusReply, error) {
	result := &coreAgent.StatusReply{}
	res, err := api.request(http.MethodGet, "status", nil, result)
	if err != nil || res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get status: %v", err)
	}

	zap.S().Infof("Agent status: %s. Connected to the Service: %s", result.Status, result.Connected)
	return result, nil
}

func (api *AgentApi) Inventory() (*v1alpha1.Inventory, error) {
	var result struct {
		Inventory v1alpha1.Inventory `json:"inventory"`
	}
	res, err := api.request(http.MethodGet, "inventory", nil, &result)
	if err != nil || res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get inventory: %v", err)
	}

	return &result.Inventory, nil
}

func (p *plannerAgentLibvirt) getConfigXmlVMPath() string {
	if p.agentEndToEndTestID == defaultAgentTestID {
		return "data/vm.xml"
	}
	return fmt.Sprintf("data/vm-%s.xml", p.agentEndToEndTestID)
}

func (p *plannerAgentLibvirt) IsoFilePath() string {
	if p.agentEndToEndTestID == defaultAgentTestID {
		return filepath.Join(defaultBasePath, "agent.iso")
	}
	fileName := fmt.Sprintf("agent-%s.iso", p.agentEndToEndTestID)
	return filepath.Join(defaultBasePath, fileName)
}

func (p *plannerAgentLibvirt) qcowDiskFilePath() string {
	if p.agentEndToEndTestID == defaultAgentTestID {
		return filepath.Join(defaultBasePath, "persistence-disk.qcow2")
	}
	fileName := fmt.Sprintf("persistence-disk-vm-%s.qcow2", p.agentEndToEndTestID)
	return filepath.Join(defaultBasePath, fileName)
}
