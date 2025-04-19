package e2e_service

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	internalclient "github.com/kubev2v/migration-planner/internal/api/client"
	. "github.com/kubev2v/migration-planner/test/e2e"
	. "github.com/kubev2v/migration-planner/test/e2e/e2e_utils"
	"go.uber.org/zap"
	"net/http"
)

type PlannerService interface {
	CreateSource(name string) (*api.Source, error)
	GetSource(id uuid.UUID) (*api.Source, error)
	RemoveSource(id uuid.UUID) error
	RemoveSources() error
	UpdateSource(uuid.UUID, *v1alpha1.Inventory) error
}

type plannerService struct {
	api *ServiceApi
}

func NewPlannerService() (*plannerService, error) {
	zap.S().Info("Initializing PlannerService...")
	return &plannerService{api: NewServiceApi()}, nil
}

func (s *plannerService) CreateSource(name string) (*api.Source, error) {
	zap.S().Info("Creating source...")
	user, err := DefaultUserAuth()
	if err != nil {
		return nil, err
	}

	params := &v1alpha1.CreateSourceJSONRequestBody{Name: name}

	if TestOptions.DisconnectedEnvironment { // make the service unreachable

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

	res, err := s.api.PostRequest("", reqBody, user.Token.Raw)
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
	user, err := DefaultUserAuth()
	if err != nil {
		return nil, err
	}

	res, err := s.api.GetRequest(id.String(), user.Token.Raw)
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

func (s *plannerService) RemoveSource(uuid uuid.UUID) error {
	user, err := DefaultUserAuth()
	if err != nil {
		return err
	}

	res, err := s.api.DeleteRequest(uuid.String(), user.Token.Raw)
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

func (s *plannerService) RemoveSources() error {
	user, err := DefaultUserAuth()
	if err != nil {
		return err
	}

	res, err := s.api.DeleteRequest("", user.Token.Raw)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to delete sources. response status code: %d", res.StatusCode)
	}

	return err
}

func (s *plannerService) UpdateSource(uuid uuid.UUID, inventory *v1alpha1.Inventory) error {
	user, err := DefaultUserAuth()
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

	res, err := s.api.PutRequest(uuid.String(), reqBody, user.Token.Raw)
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
