package v1alpha1

import (
	"context"
	"fmt"

	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/handlers/v1alpha1/mappers"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/pkg/estimations/engines"
	"github.com/kubev2v/migration-planner/pkg/estimations/estimation"
	"github.com/kubev2v/migration-planner/pkg/log"
)

// (POST /api/v1/assessments/{id}/complexity-estimation)
func (h *ServiceHandler) CalculateMigrationComplexity(ctx context.Context, request server.CalculateMigrationComplexityRequestObject) (server.CalculateMigrationComplexityResponseObject, error) {
	logger := log.NewDebugLogger("complexity_handler").
		WithContext(ctx).
		Operation("calculate_migration_complexity").
		WithUUID("assessment_id", request.Id).
		Build()

	user := auth.MustHaveUser(ctx)
	logger.Step("extract_user").WithString("org_id", user.Organization).WithString("username", user.Username).Log()

	if request.Body == nil {
		logger.Error(fmt.Errorf("empty request body")).Log()
		return server.CalculateMigrationComplexity400JSONResponse{Message: "empty body"}, nil
	}

	assessmentID := request.Id
	clusterID := request.Body.ClusterId

	if clusterID == "" {
		logger.Error(fmt.Errorf("clusterId is required")).Log()
		return server.CalculateMigrationComplexity400JSONResponse{Message: "clusterId is required"}, nil
	}

	if _, err := h.assessmentSrv.GetAssessment(ctx, assessmentID); err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			logger.Error(err).WithUUID("assessment_id", assessmentID).Log()
			return server.CalculateMigrationComplexity404JSONResponse{Message: err.Error()}, nil
		case *service.ErrForbidden:
			logger.Error(err).WithUUID("assessment_id", assessmentID).Log()
			return server.CalculateMigrationComplexity403JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).WithUUID("assessment_id", assessmentID).Log()
			return server.CalculateMigrationComplexity500JSONResponse{Message: "failed to get assessment"}, nil
		}
	}

	result, err := h.estimationSrv.CalculateMigrationComplexity(ctx, assessmentID, clusterID)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			logger.Error(err).WithUUID("assessment_id", assessmentID).Log()
			return server.CalculateMigrationComplexity404JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).Log()
			return server.CalculateMigrationComplexity500JSONResponse{Message: "failed to calculate migration complexity"}, nil
		}
	}

	logger.Success().
		WithString("org_id", user.Organization).
		WithString("username", user.Username).
		Log()

	apiResponse := mappers.MigrationComplexityResultToAPI(*result)
	return server.CalculateMigrationComplexity200JSONResponse(apiResponse), nil
}

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
		return server.CalculateMigrationEstimation400JSONResponse{Message: "empty body"}, nil
	}

	assessmentID := request.Id
	clusterID := request.Body.ClusterId

	if clusterID == "" {
		logger.Error(fmt.Errorf("clusterId is required")).Log()
		return server.CalculateMigrationEstimation400JSONResponse{Message: "clusterId is required"}, nil
	}

	if _, err := h.assessmentSrv.GetAssessment(ctx, assessmentID); err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			logger.Error(err).WithUUID("assessment_id", assessmentID).Log()
			return server.CalculateMigrationEstimation404JSONResponse{Message: err.Error()}, nil
		case *service.ErrForbidden:
			logger.Error(err).WithUUID("assessment_id", assessmentID).Log()
			return server.CalculateMigrationEstimation403JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).WithUUID("assessment_id", assessmentID).Log()
			return server.CalculateMigrationEstimation500JSONResponse{Message: "failed to get assessment"}, nil
		}
	}

	// Parse optional estimation schemas from request body
	var schemas []engines.Schema
	if request.Body.EstimationSchema != nil {
		for _, s := range *request.Body.EstimationSchema {
			schemas = append(schemas, engines.Schema(s))
		}
	}

	// Parse optional user-supplied param overrides
	var userParams []estimation.Param
	if request.Body.Params != nil {
		for k, v := range *request.Body.Params {
			userParams = append(userParams, estimation.Param{Key: k, Value: v})
		}
	}

	logger.Step("calculate_estimation").
		WithUUID("assessment_id", assessmentID).
		WithString("cluster_id", clusterID).
		WithString("org_id", user.Organization).
		WithString("username", user.Username).
		Log()

	result, err := h.estimationSrv.CalculateMigrationEstimation(ctx, assessmentID, clusterID, schemas, userParams)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			logger.Error(err).WithUUID("assessment_id", assessmentID).Log()
			return server.CalculateMigrationEstimation404JSONResponse{Message: err.Error()}, nil
		case *service.ErrInvalidSchema:
			logger.Error(err).Log()
			return server.CalculateMigrationEstimation400JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).Log()
			return server.CalculateMigrationEstimation500JSONResponse{Message: "failed to calculate migration estimation"}, nil
		}
	}

	logger.Success().
		WithString("org_id", user.Organization).
		WithString("username", user.Username).
		WithInt("schema_count", len(result)).
		Log()

	apiResponse := mappers.MigrationEstimationResultToAPI(result)
	return server.CalculateMigrationEstimation200JSONResponse(apiResponse), nil
}
