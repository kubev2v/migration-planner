package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	internalclient "github.com/kubev2v/migration-planner/internal/api/client"
	"go.uber.org/zap"
)

const (
	apiV1PartnersPath        = "/api/v1/partners"
	apiV1PartnerRequestsPath = "/api/v1/partners/requests"
	apiV1CustomersPath       = "/api/v1/customers"
)

func (s *plannerService) ListPartners() (*v1alpha1.GroupList, error) {
	zap.S().Infof("[PlannerService] List partners [user: %s]", s.credentials.Username)

	res, err := s.api.GetRequest(apiV1PartnersPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	parsed, err := internalclient.ParseListPartnersResponse(res)
	if err != nil {
		return nil, fmt.Errorf("failed to list partners. status: %d, err: %v", res.StatusCode, err)
	}

	if parsed.HTTPResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list partners. bad res status code: %d. res body: %v", res.StatusCode, string(parsed.Body))
	}
	return parsed.JSON200, nil
}

func (s *plannerService) CreatePartnerRequest(partnerID uuid.UUID, req v1alpha1.PartnerRequestCreate) (*v1alpha1.PartnerRequest, int, error) {
	zap.S().Infof("[PlannerService] Create partner request [user: %s, partner: %s]", s.credentials.Username, partnerID)

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, 0, err
	}

	res, err := s.api.PostRequest(path.Join(apiV1PartnersPath, partnerID.String(), "request"), reqBody)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = res.Body.Close() }()

	parsed, err := internalclient.ParseCreatePartnerRequestResponse(res)
	if err != nil {
		return nil, res.StatusCode, fmt.Errorf("failed to create partner request. status: %d, err: %v", res.StatusCode, err)
	}

	if parsed.HTTPResponse.StatusCode != http.StatusCreated {
		return nil, parsed.HTTPResponse.StatusCode, fmt.Errorf("failed to create partner request. bad res status code: %d. res body: %v", res.StatusCode, string(parsed.Body))
	}
	return parsed.JSON201, http.StatusCreated, nil
}

func (s *plannerService) ListPartnerRequests() (*v1alpha1.PartnerRequestList, error) {
	zap.S().Infof("[PlannerService] List partner requests [user: %s]", s.credentials.Username)

	res, err := s.api.GetRequest(apiV1PartnerRequestsPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	parsed, err := internalclient.ParseListPartnerRequestsResponse(res)
	if err != nil {
		return nil, fmt.Errorf("failed to list partner requests. status: %d, err: %v", res.StatusCode, err)
	}

	if parsed.HTTPResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list partner requests. bad res status code: %d. res body: %v", res.StatusCode, string(parsed.Body))
	}
	return parsed.JSON200, nil
}

func (s *plannerService) CancelPartnerRequest(requestID uuid.UUID) error {
	zap.S().Infof("[PlannerService] Cancel partner request [user: %s, request: %s]", s.credentials.Username, requestID)

	res, err := s.api.DeleteRequest(path.Join(apiV1PartnerRequestsPath, requestID.String()))
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("failed to cancel partner request. status: %d, body: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

func (s *plannerService) UpdatePartnerRequest(requestID uuid.UUID, req v1alpha1.PartnerRequestUpdate) (*v1alpha1.PartnerRequest, int, error) {
	zap.S().Infof("[PlannerService] Update partner request [user: %s, request: %s]", s.credentials.Username, requestID)

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, 0, err
	}

	res, err := s.api.PutRequest(path.Join(apiV1PartnerRequestsPath, requestID.String()), reqBody)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = res.Body.Close() }()

	parsed, err := internalclient.ParseUpdatePartnerRequestResponse(res)
	if err != nil {
		return nil, res.StatusCode, fmt.Errorf("failed to update partner request. status: %d, err: %v", res.StatusCode, err)
	}

	if parsed.HTTPResponse.StatusCode != http.StatusOK {
		return nil, parsed.HTTPResponse.StatusCode, fmt.Errorf("failed to update partner request. bad res status code: %d. res body: %v", res.StatusCode, string(parsed.Body))
	}
	return parsed.JSON200, http.StatusOK, nil
}

func (s *plannerService) GetPartner(partnerID uuid.UUID) (*v1alpha1.Group, error) {
	zap.S().Infof("[PlannerService] Get partner [user: %s, partner: %s]", s.credentials.Username, partnerID)

	res, err := s.api.GetRequest(path.Join(apiV1PartnersPath, partnerID.String()))
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	parsed, err := internalclient.ParseGetPartnerResponse(res)
	if err != nil {
		return nil, fmt.Errorf("failed to get partner. status: %d, err: %v", res.StatusCode, err)
	}

	if parsed.HTTPResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get partner. bad res status code: %d. res body: %v", res.StatusCode, string(parsed.Body))
	}
	return parsed.JSON200, nil
}

func (s *plannerService) LeavePartner(partnerID uuid.UUID) error {
	zap.S().Infof("[PlannerService] Leave partner [user: %s, partner: %s]", s.credentials.Username, partnerID)

	res, err := s.api.DeleteRequest(path.Join(apiV1PartnersPath, partnerID.String()))
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("failed to leave partner. status: %d, body: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

func (s *plannerService) ListCustomers() (*v1alpha1.PartnerRequestList, error) {
	zap.S().Infof("[PlannerService] List customers [user: %s]", s.credentials.Username)

	res, err := s.api.GetRequest(apiV1CustomersPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	parsed, err := internalclient.ParseListCustomersResponse(res)
	if err != nil {
		return nil, fmt.Errorf("failed to list customers. status: %d, err: %v", res.StatusCode, err)
	}

	if parsed.HTTPResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list customers. bad res status code: %d. res body: %v", res.StatusCode, string(parsed.Body))
	}
	return parsed.JSON200, nil
}

func (s *plannerService) RemoveCustomer(username string) error {
	zap.S().Infof("[PlannerService] Remove customer [user: %s, customer: %s]", s.credentials.Username, username)

	res, err := s.api.DeleteRequest(path.Join(apiV1CustomersPath, username))
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("failed to remove customer. status: %d, body: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}
