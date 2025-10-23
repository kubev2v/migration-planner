package service

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/kubev2v/migration-planner/internal/auth"
	. "github.com/kubev2v/migration-planner/test/e2e"
	. "github.com/kubev2v/migration-planner/test/e2e/utils"
	"go.uber.org/zap"
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
		baseURL:    DefaultServiceUrl,
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

func (api *ServiceApi) prepareRequest(method string, path string, body []byte) (*http.Request, error) {
	var req *http.Request
	var err error

	// Always expect a full API-relative path (e.g. "/api/v1/sources/...", "/api/v1/assessments/...")
	queryPath := strings.TrimRight(fmt.Sprintf("%s%s", api.baseURL, path), "/")

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

	req.Header.Add("X-Authorization", fmt.Sprintf("Bearer %s", api.jwtToken))
	return req, nil
}

// request is a helper function that performs an HTTP request based on the method (GET, POST, PUT, DELETE),
// path, body, and JWT token, returning the HTTP response.
func (api *ServiceApi) request(method string, path string, body []byte) (*http.Response, error) {
	req, err := api.prepareRequest(method, path, body)
	if err != nil {
		return nil, fmt.Errorf("error preparing request: %v", err)
	}

	req.Header.Add("Content-Type", "application/json")

	zap.S().Infof("[Service-API] %s [Method: %s]", req.URL.String(), req.Method)

	res, err := api.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// MultipartRequest uploads a file and optional fields using multipart/form-data
func (api *ServiceApi) MultipartRequest(path, filepathStr, assessmentName string) (*http.Response, error) {
	file, err := os.Open(filepathStr)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	if err := writer.WriteField("name", assessmentName); err != nil {
		return nil, err
	}

	part, err := writer.CreateFormFile("file", filepath.Base(filepathStr))
	if err != nil {
		return nil, err
	}

	if _, err := io.Copy(part, file); err != nil {
		return nil, err
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	req, err := api.prepareRequest(http.MethodPost, path, buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("error preparing request: %v", err)
	}

	req.Header.Add("Content-Type", writer.FormDataContentType())

	return api.httpClient.Do(req)
}
