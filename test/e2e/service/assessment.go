package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	internalclient "github.com/kubev2v/migration-planner/internal/api/client"
	"go.uber.org/zap"
)

// CreateAssessment creates a new assessment per the OpenAPI spec
func (s *plannerService) CreateAssessment(name, sourceType string, sourceId *uuid.UUID, inventory *v1alpha1.Inventory) (*v1alpha1.AssessmentV2, error) {
	zap.S().Infof("[PlannerService] Create assessment [user: %s, organization: %s]", s.credentials.Username, s.credentials.Organization)

	body := v1alpha1.CreateAssessmentJSONRequestBody{
		Name:       name,
		SourceType: sourceType,
		Inventory:  inventory,
	}
	if sourceId != nil {
		sid := *sourceId
		body.SourceId = &sid
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	res, err := s.api.PostRequest(apiV2AssessmentsPath, reqBody)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	parsed, err := internalclient.ParseCreateAssessmentResponse(res)
	if err != nil {
		return nil, fmt.Errorf("failed to create assessment. status: %d, err: %v", res.StatusCode, err)
	}

	if parsed.HTTPResponse.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create assessment. bad res status code: %d. res body: %v", res.StatusCode, string(parsed.Body))
	}

	return parsed.JSON201, nil
}

func (s *plannerService) CreateAssessmentFromRvtools(name, filepath string) (*v1alpha1.AssessmentV2, error) {
	zap.S().Infof("[PlannerService] Create assessment from RVTools (async) [user: %s, organization: %s]", s.credentials.Username, s.credentials.Organization)

	// Create the async job
	job, err := s.CreateRVToolsJob(name, filepath)
	if err != nil {
		return nil, err
	}

	// Poll until completed or failed
	const maxRetries = 60
	const pollInterval = time.Second

	for i := 0; i < maxRetries; i++ {
		job, err = s.GetJob(job.Id)
		if err != nil {
			return nil, fmt.Errorf("failed to get job status: %w", err)
		}

		switch job.Status {
		case v1alpha1.JobStatusCompleted:
			if job.AssessmentId == nil {
				return nil, fmt.Errorf("job completed but no assessment ID returned")
			}
			return s.GetAssessment(job.AssessmentId.String())
		case v1alpha1.JobStatusFailed:
			errMsg := "unknown error"
			if job.Error != nil {
				errMsg = *job.Error
			}
			return nil, fmt.Errorf("job failed: %s", errMsg)
		case v1alpha1.JobStatusCancelled:
			return nil, fmt.Errorf("job was cancelled")
		case v1alpha1.JobStatusPending, v1alpha1.JobStatusParsing, v1alpha1.JobStatusValidating:
			// Still processing, continue polling
			zap.S().Infof("[PlannerService] Job %d status: %s, waiting...", job.Id, job.Status)
			time.Sleep(pollInterval)
		default:
			return nil, fmt.Errorf("unknown job status: %s", job.Status)
		}
	}

	return nil, fmt.Errorf("job timed out after %d seconds", maxRetries)
}

// CreateRVToolsJob creates a new async RVTools processing job
func (s *plannerService) CreateRVToolsJob(name, filepath string) (*v1alpha1.Job, error) {
	zap.S().Infof("[PlannerService] Create RVTools job [user: %s, organization: %s]", s.credentials.Username, s.credentials.Organization)

	res, err := s.api.MultipartRequest(apiV2AssessmentsRVToolsPath, filepath, name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	parsed, err := internalclient.ParseCreateRVToolsAssessmentV2Response(res)
	if err != nil {
		return nil, fmt.Errorf("failed to create RVTools job. status: %d, err: %v", res.StatusCode, err)
	}

	if parsed.HTTPResponse.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("failed to create RVTools job. bad res status code: %d. res body: %v", res.StatusCode, string(parsed.Body))
	}
	return parsed.JSON202, nil
}

// GetJob retrieves a job by ID
func (s *plannerService) GetJob(id int64) (*v1alpha1.Job, error) {
	res, err := s.api.GetRequest(fmt.Sprintf("%s/%d", apiV2AssessmentsJobsPath, id))
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	parsed, err := internalclient.ParseGetJobResponse(res)
	if err != nil {
		return nil, fmt.Errorf("failed to get job. status: %d, err: %v", res.StatusCode, err)
	}

	if parsed.HTTPResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get job. bad res status code: %d. res body: %v", res.StatusCode, string(parsed.Body))
	}

	return parsed.JSON200, nil
}

// CancelJob cancels a job by ID
func (s *plannerService) CancelJob(id int64) (*v1alpha1.Job, error) {
	zap.S().Infof("[PlannerService] Cancel job %d [user: %s, organization: %s]", id, s.credentials.Username, s.credentials.Organization)

	res, err := s.api.DeleteRequest(fmt.Sprintf("%s/%d", apiV2AssessmentsJobsPath, id))
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	parsed, err := internalclient.ParseCancelJobResponse(res)
	if err != nil || parsed.HTTPResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to cancel job. status: %d, err: %v", res.StatusCode, err)
	}

	if parsed.HTTPResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to cancel job. bad res status code: %d. res body: %v", res.StatusCode, string(parsed.Body))
	}
	return parsed.JSON200, nil
}

func (s *plannerService) GetAssessment(id string) (*v1alpha1.AssessmentV2, error) {
	zap.S().Infof("[PlannerService] Get assessment [user: %s, organization: %s]", s.credentials.Username, s.credentials.Organization)

	res, err := s.api.GetRequest(path.Join(apiV2AssessmentsPath, id))
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	parsed, err := internalclient.ParseGetAssessmentV2Response(res)
	if err != nil {
		return nil, fmt.Errorf("failed to get assessment. status: %d, err: %v", res.StatusCode, err)
	}

	if parsed.HTTPResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get assessment. bad res status code: %d. res body: %v", res.StatusCode, string(parsed.Body))
	}

	return parsed.JSON200, nil
}

// GetAssessments lists all assessments
func (s *plannerService) GetAssessments() ([]v1alpha1.AssessmentV2Summary, error) {
	zap.S().Infof("[PlannerService] Get assessments [user: %s, organization: %s]", s.credentials.Username, s.credentials.Organization)

	res, err := s.api.GetRequest(apiV2AssessmentsPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	parsed, err := internalclient.ParseListAssessmentsV2Response(res)
	if err != nil {
		return nil, fmt.Errorf("failed to list assessments. status: %d", res.StatusCode)
	}

	if parsed.HTTPResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list assessments. bad res status code: %d. res body: %v", res.StatusCode, string(parsed.Body))
	}
	return *parsed.JSON200, nil
}

// UpdateAssessment updates an assessment's name
func (s *plannerService) UpdateAssessment(id string, name string) (*v1alpha1.AssessmentV2, error) {
	zap.S().Infof("[PlannerService] Update assessment [user: %s, organization: %s]", s.credentials.Username, s.credentials.Organization)

	body := v1alpha1.UpdateAssessmentV2JSONRequestBody{
		Name: &name,
	}
	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	res, err := s.api.PatchRequest(path.Join(apiV2AssessmentsPath, id), reqBody)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	parsed, err := internalclient.ParseUpdateAssessmentV2Response(res)
	if err != nil {
		return nil, fmt.Errorf("failed to update assessment. status: %d", res.StatusCode)
	}

	if parsed.HTTPResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to update assessment. bad res status code: %d. res body: %v", res.StatusCode, string(parsed.Body))
	}
	return parsed.JSON200, nil
}

// ShareAssessment shares an assessment with the user's partner organization
func (s *plannerService) ShareAssessment(id string) (int, error) {
	zap.S().Infof("[PlannerService] Share assessment %s [user: %s, organization: %s]", id, s.credentials.Username, s.credentials.Organization)

	res, err := s.api.PostRequest(path.Join(apiV2AssessmentsPath, id, "share"), nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, fmt.Errorf("failed to share assessment. status: %d, body: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	return res.StatusCode, nil
}

// UnshareAssessment unshares an assessment from the user's partner organization
func (s *plannerService) UnshareAssessment(id string) (int, error) {
	zap.S().Infof("[PlannerService] Unshare assessment %s [user: %s, organization: %s]", id, s.credentials.Username, s.credentials.Organization)

	res, err := s.api.DeleteRequest(path.Join(apiV2AssessmentsPath, id, "share"))
	if err != nil {
		return 0, err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, fmt.Errorf("failed to unshare assessment. status: %d, body: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	return res.StatusCode, nil
}

// RemoveAssessment deletes a specific assessment by ID
func (s *plannerService) RemoveAssessment(id string) error {
	zap.S().Infof("[PlannerService] Delete assessment [user: %s, organization: %s]", s.credentials.Username, s.credentials.Organization)

	res, err := s.api.DeleteRequest(path.Join(apiV2AssessmentsPath, id))
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()

	// Spec returns 200 with Assessment body
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("failed to delete assessment. status: %d, body: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

// GetAssessmentSummary retrieves assessment metadata from the v2 list endpoint.
func (s *plannerService) GetAssessmentSummary(id string) (*v1alpha1.AssessmentV2Summary, error) {
	assessments, err := s.GetAssessments()
	if err != nil {
		return nil, err
	}
	for _, assessment := range assessments {
		if assessment.Id.String() == id {
			return &assessment, nil
		}
	}
	return nil, fmt.Errorf("assessment %s not found in v2 assessment list", id)
}

// UploadRVToolsToAssessment uploads another vCenter inventory to an assessment through the v2 API.
func (s *plannerService) UploadRVToolsToAssessment(id string, filepath string) (*v1alpha1.Job, error) {
	endpoint := path.Join(apiV2AssessmentsPath, id, "rvtools")
	res, err := s.api.MultipartPutRequest(endpoint, filepath)
	if err != nil {
		return nil, err
	}

	defer func() { _ = res.Body.Close() }()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("upload RVTools to assessment: failed to read response: %w", err)
	}
	if res.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("upload RVTools to assessment: unexpected status code: %d. response body: %s", res.StatusCode, string(body))
	}

	var job v1alpha1.Job
	if err := json.Unmarshal(body, &job); err != nil {
		return nil, fmt.Errorf("upload RVTools to assessment: failed to decode job response: %w", err)
	}
	return &job, nil
}

func (s *plannerService) GetVCenter(assessmentID string, vcenterID string) (*v1alpha1.AssessmentVCenterV2, error) {
	endpoint := path.Join(apiV2AssessmentsPath, assessmentID, "vcenters", vcenterID)
	res, err := s.api.GetRequest(endpoint)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("failed to get vCenter. status: %d, body: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}

	parsed, err := internalclient.ParseGetVcenterResponse(res)
	if err != nil {
		return nil, err
	}
	return parsed.JSON200, nil
}

func (s *plannerService) GetVCenterCluster(assessmentID, vcenterID, clusterId string) (*v1alpha1.InventoryData, error) {
	endpoint := path.Join(apiV2AssessmentsPath, assessmentID, "vcenters", vcenterID, "clusters", clusterId)
	res, err := s.api.GetRequest(endpoint)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("failed to get vCenter. status: %d, body: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}

	parsed, err := internalclient.ParseGetVcenterClusterResponse(res)
	if err != nil {
		return nil, err
	}

	return parsed.JSON200, nil
}

func (s *plannerService) DeleteVCenterInventory(assessmentID string, vcenterID string) (int, error) {
	endpoint := path.Join(apiV2AssessmentsPath, assessmentID, "vcenters", vcenterID)
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
