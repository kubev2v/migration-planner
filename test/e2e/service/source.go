package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	internalclient "github.com/kubev2v/migration-planner/internal/api/client"
	. "github.com/kubev2v/migration-planner/test/e2e"
	"go.uber.org/zap"
)

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

	res, err := s.api.PostRequest(apiV1SourcesPath, reqBody)
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
	getImageUrlPath := path.Join(apiV1SourcesPath, id.String(), "image-url")
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
	res, err := s.api.GetRequest(path.Join(apiV1SourcesPath, id.String()))
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
	res, err := s.api.GetRequest(apiV1SourcesPath)
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
	res, err := s.api.DeleteRequest(path.Join(apiV1SourcesPath, uuid.String()))
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
	res, err := s.api.DeleteRequest(apiV1SourcesPath)
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
	update := v1alpha1.UpdateInventoryJSONRequestBody{
		AgentId:   uuid,
		Inventory: *inventory,
	}

	reqBody, err := json.Marshal(update)
	if err != nil {
		return err
	}

	res, err := s.api.PutRequest(path.Join(apiV1SourcesPath, uuid.String(), "inventory"), reqBody)
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
