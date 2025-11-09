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
	zap.S().Infof("[PlannerService] Create assessment [user: %s, organization: %s]", s.credentials.Username, s.credentials.Organization)

	res, err := s.api.MultipartRequest(apiV1AssessmentsPath, filepath, name)
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

// WaitForAssessmentProcessing waits for the assessment's snapshot to be processed.
// It polls the assessment until the snapshot status is "ready" or "failed", or until timeout.
// Returns the updated assessment and any error encountered.
func (s *plannerService) WaitForAssessmentProcessing(id uuid.UUID, timeout time.Duration, pollInterval time.Duration) (*v1alpha1.Assessment, error) {
	zap.S().Infof("[PlannerService] Waiting for assessment processing [assessment: %s, timeout: %v]", id, timeout)

	deadline := time.Now().Add(timeout)

	for {
		assessment, err := s.GetAssessment(id)
		if err != nil {
			return nil, fmt.Errorf("failed to get assessment while waiting: %w", err)
		}

		for _, snapshot := range assessment.Snapshots {
			if snapshot.Status == v1alpha1.Ready {
				zap.S().Infof("[PlannerService] Assessment processing completed [assessment: %s]", id)
				return assessment, nil
			}
			if snapshot.Status == v1alpha1.Failed {
				errMsg := "unknown error"
				if snapshot.Error != nil {
					errMsg = *snapshot.Error
				}
				return assessment, fmt.Errorf("assessment processing failed: %s", errMsg)
			}
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, fmt.Errorf("timeout waiting for assessment processing after %v", timeout)
		}

		waitTime := pollInterval
		if remaining < waitTime {
			waitTime = remaining
		}

		<-time.After(waitTime)
	}
}
