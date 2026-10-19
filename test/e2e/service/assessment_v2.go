package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"

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
