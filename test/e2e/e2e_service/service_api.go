package e2e_service

import (
	"bytes"
	"fmt"
	. "github.com/kubev2v/migration-planner/test/e2e"
	"go.uber.org/zap"
	"net/http"
	"strings"
)

type ServiceApi struct {
	baseURL    string
	httpClient *http.Client
}

func NewServiceApi() *ServiceApi {
	return &ServiceApi{
		baseURL:    fmt.Sprintf("%s/api/v1/sources/", DefaultServiceUrl),
		httpClient: &http.Client{},
	}
}

func (api *ServiceApi) GetRequest(path string, token string) (*http.Response, error) {
	return api.request(http.MethodGet, path, nil, token)
}

func (api *ServiceApi) PostRequest(path string, body []byte, token string) (*http.Response, error) {
	return api.request(http.MethodPost, path, body, token)
}

func (api *ServiceApi) PutRequest(path string, body []byte, token string) (*http.Response, error) {
	return api.request(http.MethodPut, path, body, token)
}

func (api *ServiceApi) DeleteRequest(path string, token string) (*http.Response, error) {
	return api.request(http.MethodDelete, path, nil, token)
}

func (api *ServiceApi) request(method string, path string, body []byte, jwtToken string) (*http.Response, error) {
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
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", jwtToken))

	zap.S().Infof("[Service-API] %s [Method: %s]", req.URL.String(), req.Method)

	res, err := api.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	return res, nil
}
