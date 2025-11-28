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

const (
	apiV1JobsPath   = "/api/v1/assessments/jobs"
	jobPollInterval = 500 * time.Millisecond
	jobPollTimeout  = 5 * time.Minute
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
	zap.S().Infof("[PlannerService] Create assessment from RVTools [user: %s, organization: %s]", s.credentials.Username, s.credentials.Organization)

	// Step 1: Create job
	res, err := s.api.MultipartRequest(apiV1JobsPath, filepath, "")
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	parsed, err := internalclient.ParseCreateRVToolsJobResponse(res)
	if err != nil {
		return nil, fmt.Errorf("failed to parse job response: %v", err)
	}
	if parsed.HTTPResponse.StatusCode != http.StatusCreated {
		errMsg := ""
		if parsed.JSON400 != nil {
			errMsg = parsed.JSON400.Message
		}
		return nil, fmt.Errorf("failed to create job. status: %d, message: %s", res.StatusCode, errMsg)
	}

	jobID := parsed.JSON201.Id
	zap.S().Infof("[PlannerService] Job created: %d", jobID)

	// Step 2: Poll for job completion
	job, err := s.waitForJobCompletion(jobID)
	if err != nil {
		return nil, err
	}

	if job.Status != v1alpha1.Completed {
		errMsg := ""
		if job.Error != nil {
			errMsg = *job.Error
		}
		return nil, fmt.Errorf("job failed with status %s: %s", job.Status, errMsg)
	}

	// Step 3: Create assessment with job ID
	body := v1alpha1.CreateAssessmentJSONRequestBody{
		Name:       name,
		SourceType: "rvtools",
		JobId:      &jobID,
	}
	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	res2, err := s.api.PostRequest(apiV1AssessmentsPath, reqBody)
	if err != nil {
		return nil, err
	}
	defer res2.Body.Close()

	parsedAssessment, err := internalclient.ParseCreateAssessmentResponse(res2)
	if err != nil || parsedAssessment.HTTPResponse.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create assessment. status: %d, err: %v", res2.StatusCode, err)
	}
	return parsedAssessment.JSON201, nil
}

func (s *plannerService) waitForJobCompletion(jobID int64) (*v1alpha1.Job, error) {
	timeout := time.After(jobPollTimeout)
	ticker := time.NewTicker(jobPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return nil, fmt.Errorf("timeout waiting for job %d to complete", jobID)
		case <-ticker.C:
			res, err := s.api.GetRequest(fmt.Sprintf("%s/%d", apiV1JobsPath, jobID))
			if err != nil {
				return nil, err
			}

			parsed, err := internalclient.ParseGetJobResponse(res)
			res.Body.Close()
			if err != nil {
				return nil, err
			}
			if parsed.HTTPResponse.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("failed to get job. status: %d", parsed.HTTPResponse.StatusCode)
			}

			job := parsed.JSON200
			if job.Status == v1alpha1.Completed || job.Status == v1alpha1.Cancelled || job.Status == v1alpha1.Discarded {
				return job, nil
			}
			zap.S().Debugf("[PlannerService] Job %d status: %s", jobID, job.Status)
		}
	}
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
