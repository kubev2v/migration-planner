package v1alpha1

import (
	"context"

	apiServer "github.com/kubev2v/migration-planner/internal/api/server"
)

// (GET /api/v1/assessments)
func (s *ServiceHandler) ListAssessments(ctx context.Context, request apiServer.ListAssessmentsRequestObject) (apiServer.ListAssessmentsResponseObject, error) {
	return apiServer.ListAssessments500JSONResponse{Message: "not implemented yet"}, nil
}

// (POST /api/v1/assessments)
func (s *ServiceHandler) CreateAssessment(ctx context.Context, request apiServer.CreateAssessmentRequestObject) (apiServer.CreateAssessmentResponseObject, error) {
	return apiServer.CreateAssessment500JSONResponse{Message: "not implemented yet"}, nil
}

// (GET /api/v1/assessments/{id})
func (s *ServiceHandler) GetAssessment(ctx context.Context, request apiServer.GetAssessmentRequestObject) (apiServer.GetAssessmentResponseObject, error) {
	return apiServer.GetAssessment500JSONResponse{Message: "not implemented yet"}, nil
}

// (DELETE /api/v1/assessments/{id})
func (s *ServiceHandler) DeleteAssessment(ctx context.Context, request apiServer.DeleteAssessmentRequestObject) (apiServer.DeleteAssessmentResponseObject, error) {
	return apiServer.DeleteAssessment500JSONResponse{Message: "not implemented yet"}, nil
}
