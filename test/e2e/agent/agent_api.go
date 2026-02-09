package agent

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"go.uber.org/zap"
)

// AgentApi provides a client to interact with the Planner Agent API
type AgentApi struct {
	baseURL    string
	httpClient *http.Client
}

// DefaultAgentApi creates an AgentApi client with a default HTTP client that skips TLS verification
func DefaultAgentApi(agentApiBaseUrl string) *AgentApi {
	return NewAgentApi(agentApiBaseUrl, &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	})
}

// NewAgentApi creates an AgentApi client with a custom HTTP client, useful for test customization
func NewAgentApi(agentApiBaseUrl string, customHttpClient *http.Client) *AgentApi {
	return &AgentApi{
		baseURL:    agentApiBaseUrl,
		httpClient: customHttpClient,
	}
}

// request is a helper to send an HTTP request to the agent and unmarshal the response into given struct
func (a *AgentApi) request(method string, path string, body []byte, result any) (*http.Response, error) {
	var req *http.Request
	var err error

	queryPath := a.baseURL + path

	switch method {
	case http.MethodGet:
		req, err = http.NewRequest(http.MethodGet, queryPath, nil)
	case http.MethodPost:
		req, err = http.NewRequest(http.MethodPost, queryPath, bytes.NewReader(body))
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
	res, err := a.httpClient.Do(req)
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

// Status retrieves the current status of the agent
func (a *AgentApi) Status() (*AgentStatus, error) {
	result := &AgentStatus{}
	res, err := a.request(http.MethodGet, "agent", nil, result)
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %v", err)
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(" unexpected response code %d", res.StatusCode)
	}

	zap.S().Infof("mode: %s. Console connection: %s", result.Mode, result.ConsoleConnection)
	return result, nil
}

// Inventory retrieves the inventory data collected by the agent
func (a *AgentApi) Inventory() (*v1alpha1.Inventory, error) {
	var inv v1alpha1.Inventory

	res, err := a.request(http.MethodGet, "inventory", nil, &inv)
	if err != nil {
		return nil, fmt.Errorf("failed to get inventory: %v", err)
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(" unexpected response code %d", res.StatusCode)
	}

	return &inv, nil
}

func (a *AgentApi) SetAgentMode(mode string) (*AgentStatus, error) {
	body := AgentModeRequest{Mode: mode}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	var status AgentStatus

	res, err := a.request(http.MethodPost, "agent", data, &status)
	if err != nil {
		return nil, fmt.Errorf("failed to set agent mode: %v", err)
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(" unexpected response code %d", res.StatusCode)
	}

	return &status, nil
}

func (a *AgentApi) StartCollector(vcenterURL, username, password string) (*CollectorStatus, int, error) {
	body := CollectorStartRequest{
		URL:      vcenterURL,
		Username: username,
		Password: password,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, 1, fmt.Errorf("marshaling request: %w", err)
	}

	var status CollectorStatus

	res, err := a.request(http.MethodPost, "collector", data, &status)
	if err != nil {
		return nil, 1, fmt.Errorf("failed to start collector: %v", err)
	}

	return &status, res.StatusCode, nil
}

func (a *AgentApi) GetCollectorStatus() (*CollectorStatus, error) {
	var status CollectorStatus

	res, err := a.request(http.MethodGet, "collector", nil, &status)
	if err != nil {
		return nil, fmt.Errorf("failed to get collector status: %v", err)
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(" unexpected response code %d", res.StatusCode)
	}

	return &status, nil
}
