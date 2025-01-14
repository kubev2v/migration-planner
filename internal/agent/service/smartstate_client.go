package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/agent/config"
	"net/http"
	"net/url"
)

type SmartStateClient struct {
	ServiceUrl string
}

type SmartStateOption func(client *SmartStateClient)

func NewSmartStateClient(opts ...SmartStateOption) SmartStateClient {
	sc := SmartStateClient{
		ServiceUrl: "http://localhost:3334",
	}

	for _, opt := range opts {
		opt(&sc)
	}

	return sc
}

func WithServiceUrl(url string) SmartStateOption {
	return func(sc *SmartStateClient) {
		sc.ServiceUrl = url
	}
}

func RemoveProtocolAndRoute(rawURL string) (string, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	return parsedURL.Host, nil
}

func (sc *SmartStateClient) InitScan(creds config.Credentials) error {
	server, err := RemoveProtocolAndRoute(creds.URL)
	if err != nil {
		return err
	}
	requestData := map[string]string{
		"server":   server,
		"username": creds.Username,
		"password": creds.Password,
	}

	data, err := json.Marshal(requestData)
	if err != nil {
		return fmt.Errorf("failed to marshal request data: %w", err)
	}

	url := fmt.Sprintf("%s/init_scan", sc.ServiceUrl)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		fmt.Println("Request succeeded with status:", resp.StatusCode)
		return nil
	}

	return fmt.Errorf("request failed with status: %d", resp.StatusCode)
}

func (sc *SmartStateClient) GetResults() (v1alpha1.SmartState, error) {
	url := fmt.Sprintf("%s/results", sc.ServiceUrl)
	req, err := http.NewRequest("GET", url, nil) // No need for bytes.NewBuffer() for a GET request
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 202 {
		fmt.Println("Request in progress. Status:", resp.StatusCode)
		return nil, nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("request failed with status: %d", resp.StatusCode)
	}

	var results []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("failed to decode JSON response: %w", err)
	}

	fmt.Println("Request succeeded with status:", resp.StatusCode)
	return results, nil
}
