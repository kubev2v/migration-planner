package v1alpha1

import (
	"context"
	"fmt"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/handlers/v1alpha1/mappers"
	"github.com/kubev2v/migration-planner/internal/handlers/validator"
	"github.com/kubev2v/migration-planner/internal/service"
	srvMappers "github.com/kubev2v/migration-planner/internal/service/mappers"
)

// (GET /api/v1/assessments)
func (h *ServiceHandler) ListAssessments(ctx context.Context, request server.ListAssessmentsRequestObject) (server.ListAssessmentsResponseObject, error) {
	user := auth.MustHaveUser(ctx)

	filter := service.NewAssessmentFilter(user.Organization)

	assessments, err := h.assessmentSrv.ListAssessments(ctx, filter)
	if err != nil {
		return server.ListAssessments500JSONResponse{Message: fmt.Sprintf("failed to list assessments: %v", err)}, nil
	}

	return server.ListAssessments200JSONResponse(mappers.AssessmentListToApi(assessments)), nil
}

// (POST /api/v1/assessments)
func (h *ServiceHandler) CreateAssessment(ctx context.Context, request server.CreateAssessmentRequestObject) (server.CreateAssessmentResponseObject, error) {
	user := auth.MustHaveUser(ctx)

	if request.JSONBody == nil && request.MultipartBody == nil {
		return server.CreateAssessment400JSONResponse{Message: "empty body"}, nil
	}

	var createForm srvMappers.AssessmentCreateForm
	createForm.OrgID = user.Organization

	// Handle JSON content type
	if request.JSONBody != nil {
		form := v1alpha1.AssessmentForm(*request.JSONBody)
		if err := validateAssessmentData(form); err != nil {
			return server.CreateAssessment400JSONResponse{Message: err.Error()}, nil
		}

		createForm = mappers.AssessmentFormToCreateForm(form, user.Organization)
	}

	// Handle multipart content type (RVTools upload)
	if request.MultipartBody != nil {
		var err error
		createForm, err = mappers.AssessmentCreateFormFromMultipart(request.MultipartBody, user.Organization)
		if err != nil {
			return server.CreateAssessment400JSONResponse{Message: fmt.Sprintf("failed to parse multipart form: %v", err)}, nil
		}
	}

	assessment, err := h.assessmentSrv.CreateAssessment(ctx, createForm)
	if err != nil {
		switch err.(type) {
		case *service.ErrAssessmentCreationForbidden:
			// forbidden to create assessment for sources not own by the user
			return server.CreateAssessment401JSONResponse{Message: err.Error()}, nil
		case *service.ErrSourceHasNoInventory:
			// source has no inventory
			return server.CreateAssessment400JSONResponse{Message: err.Error()}, nil
		default:
			return server.CreateAssessment500JSONResponse{Message: fmt.Sprintf("failed to create assessment: %v", err)}, nil
		}
	}

	return server.CreateAssessment201JSONResponse(mappers.AssessmentToApi(*assessment)), nil

}

// (GET /api/v1/assessments/{id})
func (h *ServiceHandler) GetAssessment(ctx context.Context, request server.GetAssessmentRequestObject) (server.GetAssessmentResponseObject, error) {
	assessmentID := request.Id

	assessment, err := h.assessmentSrv.GetAssessment(ctx, assessmentID)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.GetAssessment404JSONResponse{Message: err.Error()}, nil
		default:
			return server.GetAssessment500JSONResponse{Message: fmt.Sprintf("failed to get assessment: %v", err)}, nil
		}
	}

	user := auth.MustHaveUser(ctx)
	if user.Organization != assessment.OrgID {
		message := fmt.Sprintf("forbidden to access assessment %s by user with org_id %s", assessmentID, user.Organization)
		return server.GetAssessment403JSONResponse{Message: message}, nil
	}

	return server.GetAssessment200JSONResponse(mappers.AssessmentToApi(*assessment)), nil
}

// (PUT /api/v1/assessments/{id})
func (h *ServiceHandler) UpdateAssessment(ctx context.Context, request server.UpdateAssessmentRequestObject) (server.UpdateAssessmentResponseObject, error) {
	if request.Body == nil {
		return server.UpdateAssessment400JSONResponse{Message: "empty body"}, nil
	}

	assessmentID := request.Id

	// Get assessment to check ownership
	assessment, err := h.assessmentSrv.GetAssessment(ctx, assessmentID)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.UpdateAssessment404JSONResponse{Message: err.Error()}, nil
		default:
			return server.UpdateAssessment500JSONResponse{Message: fmt.Sprintf("failed to get assessment: %v", err)}, nil
		}
	}

	user := auth.MustHaveUser(ctx)
	if user.Organization != assessment.OrgID {
		message := fmt.Sprintf("forbidden to update assessment %s by user with org_id %s", assessmentID, user.Organization)
		return server.UpdateAssessment403JSONResponse{Message: message}, nil
	}

	updatedAssessment, err := h.assessmentSrv.UpdateAssessment(ctx, assessmentID, request.Body.Name)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.UpdateAssessment404JSONResponse{Message: err.Error()}, nil
		case *service.ErrAgentUpdateForbidden:
			return server.UpdateAssessment400JSONResponse{Message: err.Error()}, nil
		default:
			return server.UpdateAssessment500JSONResponse{Message: fmt.Sprintf("failed to update assessment: %v", err)}, nil
		}
	}

	return server.UpdateAssessment200JSONResponse(mappers.AssessmentToApi(*updatedAssessment)), nil
}

// (DELETE /api/v1/assessments/{id})
func (h *ServiceHandler) DeleteAssessment(ctx context.Context, request server.DeleteAssessmentRequestObject) (server.DeleteAssessmentResponseObject, error) {
	assessmentID := request.Id

	// Get assessment to check ownership
	assessment, err := h.assessmentSrv.GetAssessment(ctx, assessmentID)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.DeleteAssessment404JSONResponse{Message: err.Error()}, nil
		default:
			return server.DeleteAssessment500JSONResponse{Message: fmt.Sprintf("failed to get assessment: %v", err)}, nil
		}
	}

	user := auth.MustHaveUser(ctx)
	if user.Organization != assessment.OrgID {
		message := fmt.Sprintf("forbidden to delete assessment %s by user with org_id %s", assessmentID, user.Organization)
		return server.DeleteAssessment403JSONResponse{Message: message}, nil
	}

	if err := h.assessmentSrv.DeleteAssessment(ctx, assessmentID); err != nil {
		return server.DeleteAssessment500JSONResponse{Message: fmt.Sprintf("failed to delete assessment: %v", err)}, nil
	}

	return server.DeleteAssessment200JSONResponse{}, nil
}

func validateAssessmentData(data interface{}) error {
	v := validator.NewValidator()
	v.Register(validator.NewAssessmentValidationRules()...)
	v.RegisterStructValidation(validator.AssessmentFormValidator(), v1alpha1.AssessmentForm{})
	return v.Struct(data)
}
