package e2e_service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	internalclient "github.com/kubev2v/migration-planner/internal/api/client"
	"github.com/kubev2v/migration-planner/internal/auth"
	. "github.com/kubev2v/migration-planner/test/e2e"
	. "github.com/kubev2v/migration-planner/test/e2e/e2e_utils"
	"go.uber.org/zap"
)

// PlannerService defines the interface for interacting with the planner service
type PlannerService interface {
	CreateSource(string) (*api.Source, error)
	GetImageUrl(uuid.UUID) (string, error)
	GetSource(uuid.UUID) (*api.Source, error)
	GetSources() (*api.SourceList, error)
	RemoveSource(uuid.UUID) error
	RemoveSources() error
	UpdateSource(uuid.UUID, *v1alpha1.Inventory) error
	ChangeCredentials(user *auth.User) error
}

// plannerService is the concrete implementation of PlannerService
type plannerService struct {
	api         *ServiceApi
	credentials *auth.User
}

// DefaultPlannerService initializes a planner service using default *auth.User credentials
func DefaultPlannerService() (*plannerService, error) {
	return NewPlannerService(DefaultUserAuth())
}

// NewPlannerService initializes the planner service with custom *auth.User credentials
func NewPlannerService(cred *auth.User) (*plannerService, error) {
	zap.S().Info("Initializing PlannerService...")
	serviceApi, err := NewServiceApi(cred)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize planner service API")
	}
	return &plannerService{api: serviceApi, credentials: cred}, nil
}

// CreateSource sends a request to create a new source with the given name
func (s *plannerService) CreateSource(name string) (*api.Source, error) {
	zap.S().Infof("[PlannerService] Creating source: user: %s, organization: %s",
		s.credentials.Username, s.credentials.Organization)

	params := &v1alpha1.CreateSourceJSONRequestBody{Name: name}

	if TestOptions.DisconnectedEnvironment { // make the service unreachable

		toStrPtr := func(s string) *string {
			return &s
		}

		params.Proxy = &api.AgentProxy{
			HttpUrl:  toStrPtr("http://127.0.0.1"),
			HttpsUrl: toStrPtr("https://127.0.0.1"),
			NoProxy:  toStrPtr("vcenter.com"),
		}
	}

	reqBody, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	res, err := s.api.PostRequest("", reqBody)
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

// GetImageUrl retrieves the image URL for a specific source by UUID
func (s *plannerService) GetImageUrl(id uuid.UUID) (string, error) {
	zap.S().Infof("[PlannerService] Get image url [user: %s, organization: %s]",
		s.credentials.Username, s.credentials.Organization)
	getImageUrlPath := id.String() + "/" + "image-url"
	res, err := s.api.GetRequest(getImageUrlPath)
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

// GetSource fetches a single source by UUID
func (s *plannerService) GetSource(id uuid.UUID) (*api.Source, error) {
	zap.S().Infof("[PlannerService] Get source [user: %s, organization: %s]",
		s.credentials.Username, s.credentials.Organization)
	res, err := s.api.GetRequest(id.String())
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

// GetSources retrieves a list of all available sources
func (s *plannerService) GetSources() (*api.SourceList, error) {
	zap.S().Infof("[PlannerService] Get sources [user: %s, organization: %s]",
		s.credentials.Username, s.credentials.Organization)
	res, err := s.api.GetRequest("")
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	getSourceRes, err := internalclient.ParseListSourcesResponse(res)
	if err != nil || getSourceRes.HTTPResponse.StatusCode != 200 {
		return nil, fmt.Errorf("failed to list sources. response status code: %d", getSourceRes.HTTPResponse.StatusCode)
	}

	return getSourceRes.JSON200, nil
}

// RemoveSource deletes a specific source by UUID
func (s *plannerService) RemoveSource(uuid uuid.UUID) error {
	zap.S().Infof("[PlannerService] Delete source [user: %s, organization: %s]",
		s.credentials.Username, s.credentials.Organization)
	res, err := s.api.DeleteRequest(uuid.String())
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

// RemoveSources deletes all existing sources
func (s *plannerService) RemoveSources() error {
	zap.S().Infof("[PlannerService] Delete sources [user: %s, organization: %s]",
		s.credentials.Username, s.credentials.Organization)
	res, err := s.api.DeleteRequest("")
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to delete sources. response status code: %d", res.StatusCode)
	}

	return err
}

// UpdateSource updates the inventory of a specific source
func (s *plannerService) UpdateSource(uuid uuid.UUID, inventory *v1alpha1.Inventory) error {
	zap.S().Infof("[PlannerService] Update source [user: %s, organization: %s]",
		s.credentials.Username, s.credentials.Organization)
	update := v1alpha1.UpdateSourceJSONRequestBody{
		AgentId:   uuid,
		Inventory: *inventory,
	}

	reqBody, err := json.Marshal(update)
	if err != nil {
		return err
	}

	res, err := s.api.PutRequest(uuid.String(), reqBody)
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

// ChangeCredentials sets the plannerService's credentials and initializes a new Service API instance
// if the provided credentials differ from the currently stored ones.
func (s *plannerService) ChangeCredentials(newCredentials *auth.User) error {
	if s.credentials.Organization == newCredentials.Organization &&
		s.credentials.Username == newCredentials.Username {
		return nil
	}
	newApi, err := NewServiceApi(newCredentials)
	if err != nil {
		return fmt.Errorf("error creating new Service api instance %s", err)
	}
	s.api = newApi
	s.credentials = newCredentials
	return nil
}
