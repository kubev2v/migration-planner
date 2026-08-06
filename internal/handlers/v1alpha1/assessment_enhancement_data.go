package v1alpha1

import (
	"context"
	"fmt"

	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/handlers/validator"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/pkg/log"
)

// validateEnhancementData validates the enhancement data using the generated struct's validate tags
func validateEnhancementData(data interface{}) error {
	v := validator.NewValidator()
	return validator.TransformValidationError(v.Struct(data))
}

// (POST /api/v1/assessments/{id}/enhancement-data)
func (h *ServiceHandler) SaveAssessmentEnhancementData(ctx context.Context, request server.SaveAssessmentEnhancementDataRequestObject) (server.SaveAssessmentEnhancementDataResponseObject, error) {
	logger := log.NewDebugLogger("enhancement_data_handler").
		WithContext(ctx).
		Operation("save_enhancement_data").
		WithUUID("assessment_id", request.Id).
		Build()

	_, err := h.assessmentSrv.GetAssessment(ctx, request.Id)
	if err != nil {
		switch err.(type) {
		case *service.ErrForbidden:
			logger.Error(err).Log()
			return server.SaveAssessmentEnhancementData403JSONResponse{Message: err.Error()}, nil
		case *service.ErrResourceNotFound:
			logger.Error(err).Log()
			return server.SaveAssessmentEnhancementData404JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).Log()
			return server.SaveAssessmentEnhancementData500JSONResponse{Message: fmt.Sprintf("failed to get assessment: %v", err)}, nil
		}
	}

	if err := validateEnhancementData(*request.Body); err != nil {
		logger.Error(err).Log()
		return server.SaveAssessmentEnhancementData400JSONResponse{Message: err.Error()}, nil
	}

	stored, err := h.enhancementDataSrv.Save(ctx, request.Id, *request.Body)
	if err != nil {
		logger.Error(err).Log()
		return server.SaveAssessmentEnhancementData500JSONResponse{Message: fmt.Sprintf("failed to save enhancement data: %v", err)}, nil
	}

	return server.SaveAssessmentEnhancementData200JSONResponse(*stored), nil
}

// (GET /api/v1/assessments/{id}/enhancement-data)
func (h *ServiceHandler) GetAssessmentEnhancementData(ctx context.Context, request server.GetAssessmentEnhancementDataRequestObject) (server.GetAssessmentEnhancementDataResponseObject, error) {
	logger := log.NewDebugLogger("enhancement_data_handler").
		WithContext(ctx).
		Operation("get_enhancement_data").
		WithUUID("assessment_id", request.Id).
		Build()

	_, err := h.assessmentSrv.GetAssessment(ctx, request.Id)
	if err != nil {
		switch err.(type) {
		case *service.ErrForbidden:
			logger.Error(err).Log()
			return server.GetAssessmentEnhancementData403JSONResponse{Message: err.Error()}, nil
		case *service.ErrResourceNotFound:
			logger.Error(err).Log()
			return server.GetAssessmentEnhancementData404JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).Log()
			return server.GetAssessmentEnhancementData500JSONResponse{Message: fmt.Sprintf("failed to get assessment: %v", err)}, nil
		}
	}

	stored, err := h.enhancementDataSrv.Get(ctx, request.Id)
	if err != nil {
		logger.Error(err).Log()
		return server.GetAssessmentEnhancementData500JSONResponse{Message: fmt.Sprintf("failed to get enhancement data: %v", err)}, nil
	}

	return server.GetAssessmentEnhancementData200JSONResponse(*stored), nil
}
