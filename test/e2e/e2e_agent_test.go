package e2e_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/kubev2v/migration-planner/internal/agent/common"
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

	coreAgent "github.com/kubev2v/migration-planner/internal/agent"
)

const (
	vmName              string = "coreos-vm"
	defaultUsername     string = "admin"
	defaultOrganization string = "admin"
)

var (
	home               string = os.Getenv("HOME")
	defaultConfigPath  string = filepath.Join(home, ".config/planner/client.yaml")
	defaultBasePath    string = "/tmp/untarova/"
	defaultVmdkName           = filepath.Join(defaultBasePath, "persistence-disk.vmdk")
	defaultOvaPath     string = filepath.Join(home, "myimage.ova")
	defaultServiceUrl  string = fmt.Sprintf("http://%s:3443", os.Getenv("PLANNER_IP"))
	defaultAgentTestID string = "1"
	privateKeyPath     string = filepath.Join(os.Getenv("E2E_PRIVATE_KEY_FOLDER_PATH"), "private-key")
)

type PlannerAgent interface {
	AgentApi() (*AgentApi, error)
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

type PlannerService interface {
	RemoveSources() error
	RemoveSource(id uuid.UUID) error
	GetSource(id uuid.UUID) (*api.Source, error)
	CreateSource(name string) (*api.Source, error)
	UpdateSource(uuid.UUID, *v1alpha1.Inventory) error
}

type plannerService struct {
	c *internalclient.ClientWithResponses
}

type plannerAgentLibvirt struct {
	c                   *internalclient.ClientWithResponses
	name                string
	con                 *libvirt.Connect
	sourceID            uuid.UUID
	agentEndToEndTestID string
	localApi            *AgentApi
}

type AgentApi struct {
	baseURL    string
	httpClient *http.Client
}

func NewPlannerAgent(configPath string, sourceID uuid.UUID, name string, idForTest string) (*plannerAgentLibvirt, error) {
	_ = createConfigFile(configPath)

	c, err := client.NewFromConfigFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}

	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
		return nil, fmt.Errorf("failed to connect to hypervisor: %v", err)
	}

	return &plannerAgentLibvirt{c: c, name: name, con: conn, sourceID: sourceID, agentEndToEndTestID: idForTest}, nil
}

func (p *plannerAgentLibvirt) AgentApi() (*AgentApi, error) {
	if p.localApi != nil {
		return p.localApi, nil
	}
	agentIP, err = p.GetIp()
	if err != nil {
		return nil, err
	}
	p.localApi = NewAgentApi(fmt.Sprintf("https://%s:3333/api/v1/", agentIP))
	return p.localApi, nil
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
	ctx := contextWithJWT(auth.NewTokenContext(context.TODO(), user), user.Token.Raw)
	var res *http.Response

	if testOptions.downloadImageByUrl {
		url, err := p.getDownloadURL(ctx)
		if err != nil {
			return err
		}

		res, err = http.Get(url) // Download OVA from the extracted URL
		if err != nil {
			return err
		}
	} else {
		res, err = p.c.GetImage(ctx, p.sourceID, attachJWT) // Stream the OVA directly to res
		if err != nil {
			return fmt.Errorf("failed to get source image: %w", err)
		}
	}

	defer res.Body.Close()

	if _, err = io.Copy(ovaFile, res.Body); err != nil {
		return fmt.Errorf("failed to write to the file: %w", err)
	}

	if err := p.ovaValidateAndExtract(ovaFile); err != nil {
		return err
	}

	return nil
}

func (p *plannerAgentLibvirt) getDownloadURL(ctx context.Context) (string, error) {
	res, err := p.c.GetSourceDownloadURL(ctx, p.sourceID, attachJWT)
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

	// Remove the ISO file if it exists
	isoPath := p.IsoFilePath()
	if _, err := os.Stat(isoPath); err == nil {
		if err := os.Remove(isoPath); err != nil {
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

func NewAgentApi(baseURL string) *AgentApi {
	return &AgentApi{
		baseURL: baseURL,
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		},
	}
}

func (api *AgentApi) request(path string, result any) error {
	res, err := api.httpClient.Get(api.baseURL + path)
	if err != nil {
		return fmt.Errorf("error getting response from local server: %v", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %v", err)
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("error decoding JSON: %v", err)
	}

	return nil
}

func (api *AgentApi) Version() (string, error) {
	var result struct {
		Version string `json:"version"`
	}
	if err := api.request("version", &result); err != nil {
		return "", err
	}
	return result.Version, nil
}

func (api *AgentApi) Login(url string, user string, pass string) (*http.Response, error) {

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
		api.baseURL+"credentials",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	resp.Header.Set("Content-Type", "application/json")

	response, err := api.httpClient.Do(resp)
	if err != nil {
		return response, fmt.Errorf("failed to send request: %w", err)
	}
	defer response.Body.Close()

	return response, nil
}

func (api *AgentApi) Status() (*coreAgent.StatusReply, error) {
	result := &coreAgent.StatusReply{}
	if err := api.request("status", result); err != nil {
		return nil, err
	}

	return result, nil
}

func (api *AgentApi) Inventory() (*v1alpha1.Inventory, error) {
	var result struct {
		Inventory v1alpha1.Inventory `json:"inventory"`
	}

	if err := api.request("inventory", &result); err != nil {
		return nil, err
	}

	return &result.Inventory, nil
}

func NewPlannerService(configPath string) (*plannerService, error) {
	_ = createConfigFile(configPath)
	c, err := client.NewFromConfigFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return &plannerService{c: c}, nil
}

func (s *plannerService) CreateSource(name string) (*api.Source, error) {
	user, err := defaultUserAuth()
	if err != nil {
		return nil, err
	}
	ctx := contextWithJWT(auth.NewTokenContext(context.TODO(), user), user.Token.Raw)

	params := v1alpha1.CreateSourceJSONRequestBody{Name: name}

	if testOptions.disconnectedEnvironment { // make the service unreachable

		toStrPtr := func(s string) *string {
			return &s
		}

		params.Proxy = &api.AgentProxy{
			HttpUrl:  toStrPtr("127.0.0.1"),
			HttpsUrl: toStrPtr("127.0.0.1"),
			NoProxy:  toStrPtr("vcenter.com"),
		}
	}

	res, err := s.c.CreateSourceWithResponse(ctx, params, attachJWT)
	if err != nil || res.HTTPResponse.StatusCode != 201 {
		return nil, fmt.Errorf("failed to create the source: %v", err)
	}

	if res.JSON201 == nil {
		return nil, fmt.Errorf("failed to create the source")
	}

	return res.JSON201, nil
}

func (s *plannerService) GetSource(id uuid.UUID) (*api.Source, error) {
	user, err := defaultUserAuth()
	if err != nil {
		return nil, err
	}
	ctx := contextWithJWT(auth.NewTokenContext(context.TODO(), user), user.Token.Raw)

	res, err := s.c.GetSourceWithResponse(ctx, id, attachJWT)
	if err != nil || res.HTTPResponse.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list sources. response status code: %d", res.HTTPResponse.StatusCode)
	}

	return res.JSON200, nil
}

func (s *plannerService) RemoveSources() error {
	user, err := defaultUserAuth()
	if err != nil {
		return err
	}
	ctx := contextWithJWT(auth.NewTokenContext(context.TODO(), user), user.Token.Raw)

	_, err = s.c.DeleteSourcesWithResponse(ctx, attachJWT)
	return err
}

func (s *plannerService) RemoveSource(uuid uuid.UUID) error {
	user, err := defaultUserAuth()
	if err != nil {
		return err
	}
	ctx := contextWithJWT(auth.NewTokenContext(context.TODO(), user), user.Token.Raw)

	_, err = s.c.DeleteSourceWithResponse(ctx, uuid, attachJWT)
	return err
}

func (s *plannerService) UpdateSource(uuid uuid.UUID, inventory *v1alpha1.Inventory) error {
	update := v1alpha1.UpdateSourceJSONRequestBody{
		AgentId:   uuid,
		Inventory: *inventory,
	}

	user, err := defaultUserAuth()
	if err != nil {
		return err
	}
	ctx := contextWithJWT(auth.NewTokenContext(context.TODO(), user), user.Token.Raw)

	_, err = s.c.UpdateSourceWithResponse(ctx, uuid, update, attachJWT)
	return err
}

func createConfigFile(configPath string) error {
	// Ensure the directory exists
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory structure: %w", err)
	}

	// Create configuration
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return os.WriteFile(configPath, []byte(fmt.Sprintf("service:\n  server: %s", defaultServiceUrl)), 0644)
	}

	return nil
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

func defaultUserAuth() (*auth.User, error) {
	tokenVal, err := getToken(defaultUsername, defaultOrganization)
	if err != nil {
		return nil, fmt.Errorf("unable to create user: %v", err)
	}
	token := jwt.New(jwt.SigningMethodRS256)
	token.Raw = tokenVal
	return &auth.User{
		Username:     defaultUsername,
		Organization: defaultOrganization,
		Token:        token,
	}, nil
}

func attachJWT(ctx context.Context, req *http.Request) error {
	if jwt, found := jwtFromContext(ctx); found {
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", jwt))
	}
	return nil
}

func jwtFromContext(ctx context.Context) (string, bool) {
	val := ctx.Value(common.JwtKey)
	if val == nil {
		return "", false
	}
	return val.(string), true
}

func contextWithJWT(ctx context.Context, jwt string) context.Context {
	return context.WithValue(ctx, common.JwtKey, jwt)
}
