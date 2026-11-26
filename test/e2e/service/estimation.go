package service

import (
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"go.uber.org/zap"
)

func (s *plannerService) CalculateMigrationComplexity(assessmentID string, clusterID string) (int, error) {
	return s.postEstimation(
		path.Join(apiV2AssessmentsPath, assessmentID, "complexity-estimation"),
		v1alpha1.MigrationComplexityRequest{ClusterId: &clusterID},
	)
}

func (s *plannerService) CalculateMigrationEstimation(assessmentID string, clusterID string) (int, error) {
	return s.postEstimation(
		path.Join(apiV2AssessmentsPath, assessmentID, "migration-estimation"),
		v1alpha1.MigrationEstimationRequest{ClusterId: &clusterID},
	)
}

func (s *plannerService) CalculateMigrationEstimationByComplexity(assessmentID string, clusterID string) (int, error) {
	return s.postEstimation(
		path.Join(apiV2AssessmentsPath, assessmentID, "migration-estimation", "by-complexity"),
		v1alpha1.MigrationEstimationRequest{ClusterId: &clusterID},
	)
}

func (s *plannerService) postEstimation(apiPath string, body any) (int, error) {
	zap.S().Infof("[PlannerService] POST %s [user: %s, organization: %s]", apiPath, s.credentials.Username, s.credentials.Organization)

	reqBody, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}

	res, err := s.api.PostRequest(apiPath, reqBody)
	if err != nil {
		return 0, err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, fmt.Errorf("estimation request failed. status: %d, body: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}

	return res.StatusCode, nil
}
