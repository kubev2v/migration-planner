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
)

// CreateRVToolsV2Job creates an asynchronous assessment job through the v2 API.
func (s *plannerService) CreateRVToolsV2Job(name, filepath string) (*v1alpha1.Job, error) {
	res, err := s.api.MultipartRequest(apiV2AssessmentsRVToolsPath, filepath, name)
	if err != nil {
		return nil, err
	}
	return parseAcceptedRVToolsJob(res, "create v2 RVTools assessment job")
}

// UploadRVToolsToAssessment uploads another vCenter inventory to an assessment through the v2 API.
func (s *plannerService) UploadRVToolsToAssessment(id uuid.UUID, filepath string) (*v1alpha1.Job, error) {
	endpoint := path.Join(apiV2AssessmentsPath, id.String(), "rvtools")
	res, err := s.api.MultipartPutRequest(endpoint, filepath)
	if err != nil {
		return nil, err
	}
	return parseAcceptedRVToolsJob(res, "upload RVTools to assessment")
}

// GetAssessmentV2 retrieves the aggregate inventory exposed by the v2 API.
func (s *plannerService) GetAssessmentV2(id uuid.UUID) (*v1alpha1.AssessmentV2, error) {
	res, err := s.api.GetRequest(path.Join(apiV2AssessmentsPath, id.String()))
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read v2 assessment response: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get v2 assessment. status code: %d. response body: %s", res.StatusCode, string(body))
	}

	var assessment v1alpha1.AssessmentV2
	if err := json.Unmarshal(body, &assessment); err != nil {
		return nil, fmt.Errorf("failed to decode v2 assessment response: %w", err)
	}
	return &assessment, nil
}

func (s *plannerService) GetVCenterV2(assessmentID, vcenterID uuid.UUID) (int, error) {
	endpoint := path.Join(apiV2AssessmentsPath, assessmentID.String(), "vcenters", vcenterID.String())
	res, err := s.api.GetRequest(endpoint)
	if err != nil {
		return 0, err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return res.StatusCode, fmt.Errorf("failed to get vCenter. status: %d, body: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	return res.StatusCode, nil
}

func (s *plannerService) DeleteVCenterInventory(assessmentID, vcenterID uuid.UUID) (int, error) {
	endpoint := path.Join(apiV2AssessmentsPath, assessmentID.String(), "vcenters", vcenterID.String())
	res, err := s.api.DeleteRequest(endpoint)
	if err != nil {
		return 0, err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(res.Body)
		return res.StatusCode, fmt.Errorf("failed to delete vCenter inventory. status: %d, body: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	return res.StatusCode, nil
}

func parseAcceptedRVToolsJob(res *http.Response, operation string) (*v1alpha1.Job, error) {
	defer func() { _ = res.Body.Close() }()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to read response: %w", operation, err)
	}
	if res.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("%s: unexpected status code %d. response body: %s", operation, res.StatusCode, string(body))
	}

	var job v1alpha1.Job
	if err := json.Unmarshal(body, &job); err != nil {
		return nil, fmt.Errorf("%s: failed to decode job response: %w", operation, err)
	}
	return &job, nil
}
