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
func (s *plannerService) CreateAssessment(name, sourceType string, sourceId *uuid.UUID, inventory *v1alpha1.Inventory) (*v1alpha1.Assessment, error) {
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

	res, err := s.api.PostRequest(apiV1AssessmentsPath, reqBody)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	parsed, err := internalclient.ParseCreateAssessmentResponse(res)
	if err != nil || parsed.HTTPResponse.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create assessment. status: %d, err: %v", res.StatusCode, err)
	}
	return parsed.JSON201, nil
}

func (s *plannerService) CreateAssessmentFromRvtools(name, filepath string) (*v1alpha1.Assessment, error) {
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
		case v1alpha1.Completed:
			if job.AssessmentId == nil {
				return nil, fmt.Errorf("job completed but no assessment ID returned")
			}
			return s.GetAssessment(*job.AssessmentId)
		case v1alpha1.Failed:
			errMsg := "unknown error"
			if job.Error != nil {
				errMsg = *job.Error
			}
			return nil, fmt.Errorf("job failed: %s", errMsg)
		case v1alpha1.Cancelled:
			return nil, fmt.Errorf("job was cancelled")
		case v1alpha1.Pending, v1alpha1.Parsing, v1alpha1.Validating:
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

	res, err := s.api.MultipartRequest(apiV1AssessmentsRVToolsPath, filepath, name)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	parsed, err := internalclient.ParseCreateRVToolsAssessmentResponse(res)
	if err != nil || parsed.HTTPResponse.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("failed to create RVTools job. status: %d, err: %v", res.StatusCode, err)
	}
	return parsed.JSON202, nil
}

// GetJob retrieves a job by ID
func (s *plannerService) GetJob(id int64) (*v1alpha1.Job, error) {
	res, err := s.api.GetRequest(fmt.Sprintf("%s/%d", apiV1AssessmentsJobsPath, id))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	parsed, err := internalclient.ParseGetJobResponse(res)
	if err != nil || parsed.HTTPResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get job. status: %d, err: %v", res.StatusCode, err)
	}
	return parsed.JSON200, nil
}

// CancelJob cancels a job by ID
func (s *plannerService) CancelJob(id int64) (*v1alpha1.Job, error) {
	zap.S().Infof("[PlannerService] Cancel job %d [user: %s, organization: %s]", id, s.credentials.Username, s.credentials.Organization)

	res, err := s.api.DeleteRequest(fmt.Sprintf("%s/%d", apiV1AssessmentsJobsPath, id))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	parsed, err := internalclient.ParseCancelJobResponse(res)
	if err != nil || parsed.HTTPResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to cancel job. status: %d, err: %v", res.StatusCode, err)
	}
	return parsed.JSON200, nil
}

// GetAssessment retrieves a specific assessment by ID
func (s *plannerService) GetAssessment(id uuid.UUID) (*v1alpha1.Assessment, error) {
	zap.S().Infof("[PlannerService] Get assessment [user: %s, organization: %s]", s.credentials.Username, s.credentials.Organization)

	res, err := s.api.GetRequest(path.Join(apiV1AssessmentsPath, id.String()))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	parsed, err := internalclient.ParseGetAssessmentResponse(res)
	if err != nil || parsed.HTTPResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get assessment. status: %d", res.StatusCode)
	}
	return parsed.JSON200, nil
}

// GetAssessments lists all assessments
func (s *plannerService) GetAssessments() (*v1alpha1.AssessmentList, error) {
	zap.S().Infof("[PlannerService] Get assessments [user: %s, organization: %s]", s.credentials.Username, s.credentials.Organization)

	res, err := s.api.GetRequest(apiV1AssessmentsPath)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	parsed, err := internalclient.ParseListAssessmentsResponse(res)
	if err != nil || parsed.HTTPResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list assessments. status: %d", res.StatusCode)
	}
	return parsed.JSON200, nil
}

// UpdateAssessment updates an assessment's name
func (s *plannerService) UpdateAssessment(id uuid.UUID, name string) (*v1alpha1.Assessment, error) {
	zap.S().Infof("[PlannerService] Update assessment [user: %s, organization: %s]", s.credentials.Username, s.credentials.Organization)

	body := v1alpha1.UpdateAssessmentJSONRequestBody{
		Name: &name,
	}
	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	res, err := s.api.PutRequest(path.Join(apiV1AssessmentsPath, id.String()), reqBody)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	parsed, err := internalclient.ParseUpdateAssessmentResponse(res)
	if err != nil || parsed.HTTPResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to update assessment. status: %d", res.StatusCode)
	}
	return parsed.JSON200, nil
}

// RemoveAssessment deletes a specific assessment by ID
func (s *plannerService) RemoveAssessment(id uuid.UUID) error {
	zap.S().Infof("[PlannerService] Delete assessment [user: %s, organization: %s]", s.credentials.Username, s.credentials.Organization)

	res, err := s.api.DeleteRequest(path.Join(apiV1AssessmentsPath, id.String()))
	if err != nil {
		return err
	}
	defer res.Body.Close()

	// Spec returns 200 with Assessment body
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("failed to delete assessment. status: %d, body: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}
