package v1alpha1

import (
	"context"
	"fmt"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/handlers/validator"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/pkg/log"
)

// (DELETE /api/v1/assessments/{id})
func (h *ServiceHandler) DeleteAssessment(ctx context.Context, request server.DeleteAssessmentRequestObject) (server.DeleteAssessmentResponseObject, error) {
	logger := log.NewDebugLogger("assessment_handler").
		WithContext(ctx).
		Operation("delete_assessment").
		WithUUID("assessment_id", request.Id).
		Build()

	assessmentID := request.Id

	if err := h.assessmentSrv.DeleteAssessment(ctx, assessmentID); err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			logger.Error(err).WithUUID("assessment_id", assessmentID).Log()
			return server.DeleteAssessment404JSONResponse{Message: err.Error()}, nil
		case *service.ErrForbidden:
			logger.Error(err).WithUUID("assessment_id", assessmentID).Log()
			return server.DeleteAssessment403JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).WithUUID("assessment_id", assessmentID).Log()
			return server.DeleteAssessment500JSONResponse{Message: fmt.Sprintf("failed to delete assessment: %v", err)}, nil
		}
	}

	logger.Success().WithUUID("assessment_id", assessmentID).Log()
	return server.DeleteAssessment200JSONResponse{}, nil
}

// (POST /api/v1/assessments/{id}/share)
func (h *ServiceHandler) ShareAssessment(ctx context.Context, request server.ShareAssessmentRequestObject) (server.ShareAssessmentResponseObject, error) {
	logger := log.NewDebugLogger("assessment_handler").
		WithContext(ctx).
		Operation("share_assessment").
		WithUUID("assessment_id", request.Id).
		Build()

	if err := h.assessmentSrv.ShareAssessment(ctx, request.Id); err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.ShareAssessment404JSONResponse{Message: err.Error()}, nil
		case *service.ErrForbidden:
			return server.ShareAssessment403JSONResponse{Message: err.Error()}, nil
		case *service.ErrNotACustomer:
			return server.ShareAssessment400JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).Log()
			return server.ShareAssessment500JSONResponse{Message: fmt.Sprintf("failed to share assessment: %v", err)}, nil
		}
	}

	logger.Success().Log()
	return server.ShareAssessment200JSONResponse{Message: strPtr("assessment shared with partner")}, nil
}

// (DELETE /api/v1/assessments/{id}/share)
func (h *ServiceHandler) UnshareAssessment(ctx context.Context, request server.UnshareAssessmentRequestObject) (server.UnshareAssessmentResponseObject, error) {
	logger := log.NewDebugLogger("assessment_handler").
		WithContext(ctx).
		Operation("unshare_assessment").
		WithUUID("assessment_id", request.Id).
		Build()

	if err := h.assessmentSrv.UnshareAssessment(ctx, request.Id); err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.UnshareAssessment404JSONResponse{Message: err.Error()}, nil
		case *service.ErrForbidden:
			return server.UnshareAssessment403JSONResponse{Message: err.Error()}, nil
		case *service.ErrNotACustomer:
			return server.UnshareAssessment400JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).Log()
			return server.UnshareAssessment500JSONResponse{Message: fmt.Sprintf("failed to unshare assessment: %v", err)}, nil
		}
	}

	logger.Success().Log()
	return server.UnshareAssessment200JSONResponse{Message: strPtr("assessment unshared from partner")}, nil
}

func strPtr(s string) *string { return &s }

func validateAssessmentData(data interface{}) error {
	v := validator.NewValidator()
	v.Register(validator.NewAssessmentValidationRules()...)
	v.RegisterStructValidation(validator.AssessmentFormValidator(), v1alpha1.AssessmentForm{})
	return v.Struct(data)
}
