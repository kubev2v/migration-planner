package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	internalclient "github.com/kubev2v/migration-planner/internal/api/client"
	"go.uber.org/zap"
	"net/http"
	"os"
	"strings"
)

type PlannerService interface {
	RemoveSources() error
	RemoveSource(id uuid.UUID) error
	GetSource(id uuid.UUID) (*api.Source, error)
	CreateSource(name string) (*api.Source, error)
	UpdateSource(uuid.UUID, *v1alpha1.Inventory) error
}

var (
	defaultServiceUrl string = fmt.Sprintf("http://%s:3443", os.Getenv("PLANNER_IP"))
)

type plannerService struct {
	api *ServiceApi
}

type ServiceApi struct {
	baseURL    string
	httpClient *http.Client
}

func NewPlannerService() (*plannerService, error) {
	zap.S().Info("Initializing PlannerService...")
	return &plannerService{api: NewServiceApi()}, nil
}

func (s *plannerService) CreateSource(name string) (*api.Source, error) {
	zap.S().Info("Creating source...")
	user, err := defaultUserAuth()
	if err != nil {
		return nil, err
	}

	params := &v1alpha1.CreateSourceJSONRequestBody{Name: name}

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

	reqBody, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	res, err := s.api.request(http.MethodPost, "", reqBody, user.Token.Raw)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	createSourceRes, err := internalclient.ParseCreateSourceResponse(res)
	if err != nil || createSourceRes.HTTPResponse.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create the source: %v", err)
	}

	if createSourceRes.JSON201 == nil {
		return nil, fmt.Errorf("failed to create the source")
	}

	zap.S().Info("Source created successfully")

	return createSourceRes.JSON201, nil
}

func (s *plannerService) GetSource(id uuid.UUID) (*api.Source, error) {
	user, err := defaultUserAuth()
	if err != nil {
		return nil, err
	}

	res, err := s.api.request(http.MethodGet, id.String(), nil, user.Token.Raw)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	getSourceRes, err := internalclient.ParseGetSourceResponse(res)
	if err != nil || getSourceRes.HTTPResponse.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list sources. response status code: %d", getSourceRes.HTTPResponse.StatusCode)
	}

	return getSourceRes.JSON200, nil
}

func (s *plannerService) RemoveSources() error {
	user, err := defaultUserAuth()
	if err != nil {
		return err
	}

	res, err := s.api.request(http.MethodDelete, "", nil, user.Token.Raw)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to delete sources. response status code: %d", res.StatusCode)
	}

	return err
}

func (s *plannerService) RemoveSource(uuid uuid.UUID) error {
	user, err := defaultUserAuth()
	if err != nil {
		return err
	}

	res, err := s.api.request(http.MethodDelete, uuid.String(), nil, user.Token.Raw)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to delete source with uuid: %s. "+
			"response status code: %d", uuid.String(), res.StatusCode)
	}

	return err
}

func (s *plannerService) UpdateSource(uuid uuid.UUID, inventory *v1alpha1.Inventory) error {
	user, err := defaultUserAuth()
	if err != nil {
		return err
	}

	update := v1alpha1.UpdateSourceJSONRequestBody{
		AgentId:   uuid,
		Inventory: *inventory,
	}

	reqBody, err := json.Marshal(update)
	if err != nil {
		return err
	}

	res, err := s.api.request(http.MethodPut, uuid.String(), reqBody, user.Token.Raw)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to update source with uuid: %s. "+
			"response status code: %d", uuid.String(), res.StatusCode)
	}

	return err
}

func NewServiceApi() *ServiceApi {
	return &ServiceApi{
		baseURL:    fmt.Sprintf("%s/api/v1/sources/", defaultServiceUrl),
		httpClient: &http.Client{},
	}
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
