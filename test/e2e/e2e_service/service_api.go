package e2e_service

import (
	"bytes"
	"fmt"
	"github.com/kubev2v/migration-planner/internal/auth"
	. "github.com/kubev2v/migration-planner/test/e2e"
	. "github.com/kubev2v/migration-planner/test/e2e/e2e_utils"
	"go.uber.org/zap"
	"net/http"
	"strings"
)

type ServiceApi struct {
	baseURL    string
	httpClient *http.Client
	jwtToken   string
}

// NewServiceApi creates and returns a new ServiceApi instance, initializing the base URL
// and HTTP client for making requests to the service API
func NewServiceApi(cred *auth.User) (*ServiceApi, error) {
	token, err := GetToken(cred)
	if err != nil {
		return nil, fmt.Errorf("error getting token: %v", err)
	}
	return &ServiceApi{
		baseURL:    fmt.Sprintf("%s/api/v1/sources/", DefaultServiceUrl),
		httpClient: &http.Client{},
		jwtToken:   token,
	}, nil
}

// GetRequest makes an HTTP GET request to the specified path, passing the provided token
// for authorization, and returns the HTTP response
func (api *ServiceApi) GetRequest(path string) (*http.Response, error) {
	return api.request(http.MethodGet, path, nil)
}

// PostRequest makes an HTTP POST request to the specified path with the provided body
// and authorization token, returning the HTTP response
func (api *ServiceApi) PostRequest(path string, body []byte) (*http.Response, error) {
	return api.request(http.MethodPost, path, body)
}

// PutRequest makes an HTTP PUT request to the specified path with the provided body
// and authorization token, returning the HTTP response.
func (api *ServiceApi) PutRequest(path string, body []byte) (*http.Response, error) {
	return api.request(http.MethodPut, path, body)
}

// DeleteRequest makes an HTTP DELETE request to the specified path with the provided token
// for authorization, returning the HTTP response.
func (api *ServiceApi) DeleteRequest(path string) (*http.Response, error) {
	return api.request(http.MethodDelete, path, nil)
}

// request is a helper function that performs an HTTP request based on the method (GET, POST, PUT, DELETE),
// path, body, and JWT token, returning the HTTP response.
func (api *ServiceApi) request(method string, path string, body []byte) (*http.Response, error) {
	var req *http.Request
	var err error

	queryPath := strings.TrimRight(api.baseURL+path, "/")

	switch method {
	case http.MethodGet:
		req, err = http.NewRequest(http.MethodGet, queryPath, nil)
	case http.MethodPost:
		req, err = http.NewRequest(http.MethodPost, queryPath, bytes.NewReader(body))
	case http.MethodPut:
		req, err = http.NewRequest(http.MethodPut, queryPath, bytes.NewReader(body))
	case http.MethodDelete:
		req, err = http.NewRequest(http.MethodDelete, queryPath, nil)
	default:
		return nil, fmt.Errorf("unsupported method: %s", method)
	}

	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", api.jwtToken))

	zap.S().Infof("[Service-API] %s [Method: %s]", req.URL.String(), req.Method)

	res, err := api.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	return res, nil
}
