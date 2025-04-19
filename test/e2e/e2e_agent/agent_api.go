package e2e_agent

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	coreAgent "github.com/kubev2v/migration-planner/internal/agent"
	"go.uber.org/zap"
	"io"
	"net/http"
)

type AgentApi struct {
	baseURL    string
	httpClient *http.Client
}

func DefaultAgentApi(agentApiBaseUrl string) *AgentApi {
	return &AgentApi{
		baseURL: agentApiBaseUrl,
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		},
	}
}

func CustomAgentApi(agentApiBaseUrl string, customHttpClient *http.Client) *AgentApi {
	return &AgentApi{
		baseURL:    agentApiBaseUrl,
		httpClient: customHttpClient,
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
