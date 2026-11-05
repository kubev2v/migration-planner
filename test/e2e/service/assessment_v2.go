package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	internalclient "github.com/kubev2v/migration-planner/internal/api/client"
)

// GetAssessmentSummary retrieves assessment metadata from the v2 list endpoint.
func (s *plannerService) GetAssessmentSummary(id uuid.UUID) (*v1alpha1.AssessmentV2Summary, error) {
	assessments, err := s.GetAssessments()
	if err != nil {
		return nil, err
	}
	for _, assessment := range *assessments {
		if assessment.Id == id {
			return &assessment, nil
		}
	}
	return nil, fmt.Errorf("assessment %s not found in v2 assessment list", id)
}

// GetAssessmentInventory collects the v2 aggregate and cluster inventories for e2e assertions.
func (s *plannerService) GetAssessmentInventory(id uuid.UUID) (*v1alpha1.Inventory, error) {
	assessment, err := s.GetAssessmentV2(id)
	if err != nil {
		return nil, err
	}
	inventory := &v1alpha1.Inventory{
		VcenterId:      assessment.Data.Id,
		Vcenter:        &assessment.Data.Data,
		VcenterVersion: assessment.Data.VcenterVersion,
		CreatedAt:      assessment.Data.CreatedAt,
		CollectionTime: assessment.Data.CollectionTime,
		Clusters:       make(map[string]v1alpha1.InventoryData),
	}
	for _, vcenter := range assessment.Vcenters {
		endpoint := path.Join(apiV2AssessmentsPath, id.String(), "vcenters", vcenter.Id)
		res, err := s.api.GetRequest(endpoint)
		if err != nil {
			return nil, err
		}
		parsed, err := internalclient.ParseGetVcenterResponse(res)
		_ = res.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("decode vCenter inventory: %w", err)
		}
		if parsed.JSON200 == nil {
			return nil, fmt.Errorf("get vCenter inventory: status %d", res.StatusCode)
		}
		for _, cluster := range parsed.JSON200.Clusters {
			res, err := s.api.GetRequest(path.Join(endpoint, "clusters", cluster.Id))
			if err != nil {
				return nil, err
			}
			parsed, err := internalclient.ParseGetVcenterClusterResponse(res)
			_ = res.Body.Close()
			if err != nil {
				return nil, fmt.Errorf("decode cluster inventory: %w", err)
			}
			if parsed.JSON200 == nil {
				return nil, fmt.Errorf("get cluster inventory: status %d", res.StatusCode)
			}
			inventory.Clusters[cluster.Id] = *parsed.JSON200
		}
	}
	return inventory, nil
}

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
		return nil, fmt.Errorf("%s: unexpected status code: %d. response body: %s", operation, res.StatusCode, string(body))
	}

	var job v1alpha1.Job
	if err := json.Unmarshal(body, &job); err != nil {
		return nil, fmt.Errorf("%s: failed to decode job response: %w", operation, err)
	}
	return &job, nil
}
