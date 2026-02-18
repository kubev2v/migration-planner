package v1alpha1

import (
	"context"
	"errors"
	"fmt"

	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/handlers/v1alpha1/mappers"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/pkg/log"
	"github.com/kubev2v/migration-planner/pkg/requestid"
)

// (POST /api/v1/assessments/{id}/migration-estimation)
func (h *ServiceHandler) CalculateMigrationEstimation(ctx context.Context, request server.CalculateMigrationEstimationRequestObject) (server.CalculateMigrationEstimationResponseObject, error) {
	logger := log.NewDebugLogger("estimation_handler").
		WithContext(ctx).
		Operation("calculate_migration_estimation").
		WithUUID("assessment_id", request.Id).
		Build()

	user := auth.MustHaveUser(ctx)
	logger.Step("extract_user").WithString("org_id", user.Organization).WithString("username", user.Username).Log()

	if request.Body == nil {
		logger.Error(fmt.Errorf("empty request body")).Log()
		return server.CalculateMigrationEstimation400JSONResponse{Message: "empty body", RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	assessmentID := request.Id
	clusterID := request.Body.ClusterId

	if clusterID == "" {
		logger.Error(fmt.Errorf("clusterId is required")).Log()
		return server.CalculateMigrationEstimation400JSONResponse{Message: "clusterId is required", RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	// Get assessment to verify ownership
	assessment, err := h.assessmentSrv.GetAssessment(ctx, assessmentID)
	if err != nil {
		var notFound *service.ErrResourceNotFound
		if errors.As(err, &notFound) {
			logger.Error(err).WithUUID("assessment_id", assessmentID).Log()
			return server.CalculateMigrationEstimation404JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
		}
		logger.Error(err).WithUUID("assessment_id", assessmentID).Log()
		return server.CalculateMigrationEstimation500JSONResponse{Message: "failed to get assessment", RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	// Verify user owns the assessment
	if user.Username != assessment.Username || user.Organization != assessment.OrgID {
		message := fmt.Sprintf("forbidden to access assessment %s by user %s", assessmentID, user.Username)
		logger.Error(fmt.Errorf("authorization failed: %s", message)).Log()
		return server.CalculateMigrationEstimation403JSONResponse{Message: message, RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	logger.Step("calculate_estimation").
		WithUUID("assessment_id", assessmentID).
		WithString("cluster_id", clusterID).
		WithString("org_id", user.Organization).
		WithString("username", user.Username).
		Log()

	// Call estimation service
	result, err := h.estimationSrv.CalculateMigrationEstimation(ctx, assessmentID, clusterID)
	if err != nil {
		var notFound *service.ErrResourceNotFound
		if errors.As(err, &notFound) {
			logger.Error(err).WithUUID("assessment_id", assessmentID).Log()
			return server.CalculateMigrationEstimation404JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
		}
		logger.Error(err).Log()
		return server.CalculateMigrationEstimation500JSONResponse{Message: "failed to calculate migration estimation", RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	logger.Success().
		WithString("org_id", user.Organization).
		WithString("username", user.Username).
		WithString("total_duration", result.TotalDuration.String()).
		Log()

	// Convert domain model to API response
	apiResponse := mappers.MigrationEstimationResultToAPI(*result)
	return server.CalculateMigrationEstimation200JSONResponse(apiResponse), nil
}
