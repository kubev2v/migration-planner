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
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/log"
	"github.com/kubev2v/migration-planner/pkg/requestid"
)

// (GET /api/v1/assessments)
func (h *ServiceHandler) ListAssessments(ctx context.Context, request server.ListAssessmentsRequestObject) (server.ListAssessmentsResponseObject, error) {
	logger := log.NewDebugLogger("assessment_handler").
		WithContext(ctx).
		Operation("list_assessments").
		Build()

	user := auth.MustHaveUser(ctx)
	logger.Step("extract_user").WithString("org_id", user.Organization).WithString("username", user.Username).Log()

	filter := service.NewAssessmentFilter(user.Username, user.Organization)

	assessments, err := h.assessmentSrv.ListAssessments(ctx, filter)
	if err != nil {
		logger.Error(err).Log()
		return server.ListAssessments500JSONResponse{Message: fmt.Sprintf("failed to list assessments: %v", err), RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	logger.Success().WithInt("count", len(assessments)).Log()

	apiAssessments, err := mappers.AssessmentListToApi(assessments)
	if err != nil {
		return server.ListAssessments500JSONResponse{Message: fmt.Sprintf("failed to list assessments: %v", err), RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	return server.ListAssessments200JSONResponse(apiAssessments), nil
}

// (POST /api/v1/assessments)
func (h *ServiceHandler) CreateAssessment(ctx context.Context, request server.CreateAssessmentRequestObject) (server.CreateAssessmentResponseObject, error) {
	logger := log.NewDebugLogger("assessment_handler").
		WithContext(ctx).
		Operation("create_assessment").
		WithRequestBody("request_body", request.JSONBody).
		Build()

	user := auth.MustHaveUser(ctx)
	logger.Step("extract_user").WithString("org_id", user.Organization).WithString("username", user.Username).Log()

	if request.JSONBody == nil && request.MultipartBody == nil {
		logger.Error(fmt.Errorf("empty request body")).Log()
		return server.CreateAssessment400JSONResponse{Message: "empty body", RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	var createForm srvMappers.AssessmentCreateForm
	createForm.OrgID = user.Organization

	// Handle JSON content type
	if request.JSONBody != nil {
		logger.Step("process_json_body").Log()
		form := v1alpha1.AssessmentForm(*request.JSONBody)
		if err := validateAssessmentData(form); err != nil {
			logger.Error(err).WithString("step", "validation").Log()
			return server.CreateAssessment400JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
		}

		createForm = mappers.AssessmentFormToCreateForm(form, user)
		logger.Step("mapped_json_form").WithString("source_type", createForm.Source).Log()
	}

	// Handle multipart content type (RVTools upload)
	if request.MultipartBody != nil {
		logger.Step("process_multipart_body").Log()
		var err error
		createForm, err = mappers.AssessmentCreateFormFromMultipart(request.MultipartBody, user)
		if err != nil {
			logger.Error(err).WithString("step", "parse_multipart").Log()
			return server.CreateAssessment400JSONResponse{Message: fmt.Sprintf("failed to parse multipart form: %v", err), RequestId: requestid.FromContextPtr(ctx)}, nil
		}
		logger.Step("mapped_multipart_form").WithString("source_type", createForm.Source).Log()

		// Validate RVTools file format
		if err := validator.ValidateRVToolsFile(createForm.RVToolsFile); err != nil {
			logger.Error(err).WithString("step", "validate_rvtools_file").Log()
			return server.CreateAssessment400JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
		}
		logger.Step("validated_rvtools_file").Log()
	}

	logger.Step("create_assessment").
		WithUUID("id", createForm.ID).
		WithString("name", createForm.Name).
		WithString("org_id", createForm.OrgID).
		WithString("source", createForm.Source).
		WithUUIDPtr("source_id", createForm.SourceID).
		Log()

	var assessment *model.Assessment
	var err error

	// Route to appropriate create method based on source type
	if createForm.Source == service.SourceTypeRvtools {
		assessment, err = h.assessmentSrv.CreateRvtoolsAssessment(ctx, createForm)
	} else {
		assessment, err = h.assessmentSrv.CreateAssessment(ctx, createForm)
	}
	if err != nil {
		switch err.(type) {
		case *service.ErrAssessmentCreationForbidden:
			logger.Error(err).WithString("step", "authorization").Log()
			return server.CreateAssessment401JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
		case *service.ErrSourceHasNoInventory:
			logger.Error(err).WithString("step", "inventory_check").Log()
			return server.CreateAssessment400JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
		case *service.ErrDuplicateKey:
			logger.Error(err).WithString("step", "validate_input").Log()
			return server.CreateAssessment409JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
		case *service.ErrFileCorrupted:
			logger.Error(err).WithString("step", "file_validation").Log()
			return server.CreateAssessment400JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
		default:
			logger.Error(err).Log()
			return server.CreateAssessment500JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
		}
	}

	logger.Success().
		WithUUID("assessment_id", assessment.ID).
		WithString("assessment_name", assessment.Name).
		WithString("source_type", assessment.SourceType).
		Log()

	apiAssessment, err := mappers.AssessmentToApi(*assessment)
	if err != nil {
		return server.CreateAssessment500JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	return server.CreateAssessment201JSONResponse(apiAssessment), nil
}

// (GET /api/v1/assessments/{id})
func (h *ServiceHandler) GetAssessment(ctx context.Context, request server.GetAssessmentRequestObject) (server.GetAssessmentResponseObject, error) {
	logger := log.NewDebugLogger("assessment_handler").
		WithContext(ctx).
		Operation("get_assessment").
		WithUUID("assessment_id", request.Id).
		Build()

	assessmentID := request.Id

	assessment, err := h.assessmentSrv.GetAssessment(ctx, assessmentID)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			logger.Error(err).WithUUID("assessment_id", assessmentID).Log()
			return server.GetAssessment404JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
		default:
			logger.Error(err).WithUUID("assessment_id", assessmentID).Log()
			return server.GetAssessment500JSONResponse{Message: fmt.Sprintf("failed to get assessment: %v", err), RequestId: requestid.FromContextPtr(ctx)}, nil
		}
	}

	logger.Step("assessment_retrieved").WithString("assessment_name", assessment.Name).Log()

	user := auth.MustHaveUser(ctx)
	if user.Username != assessment.Username || user.Organization != assessment.OrgID {
		message := fmt.Sprintf("forbidden to access assessment %s by user %s", assessmentID, user.Username)
		logger.Error(fmt.Errorf("authorization failed: %s", message)).WithUUID("assessment_id", assessmentID).WithString("user", user.Username).WithString("assessment_username", assessment.Username).Log()
		return server.GetAssessment403JSONResponse{Message: message, RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	logger.Step("authorization_check_passed").Log()
	logger.Success().WithString("source_type", assessment.SourceType).Log()

	apiAssessment, err := mappers.AssessmentToApi(*assessment)
	if err != nil {
		return server.GetAssessment500JSONResponse{Message: fmt.Sprintf("failed to get assessment: %v", err), RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	return server.GetAssessment200JSONResponse(apiAssessment), nil
}

// (PUT /api/v1/assessments/{id})
func (h *ServiceHandler) UpdateAssessment(ctx context.Context, request server.UpdateAssessmentRequestObject) (server.UpdateAssessmentResponseObject, error) {
	logger := log.NewDebugLogger("assessment_handler").
		WithContext(ctx).
		Operation("update_assessment").
		WithUUID("assessment_id", request.Id).
		WithRequestBody("request_body", request.Body).
		Build()

	user := auth.MustHaveUser(ctx)
	logger.Step("extract_user").WithString("org_id", user.Organization).WithString("username", user.Username).Log()

	if request.Body == nil {
		logger.Error(fmt.Errorf("empty request body")).WithUUID("assessment_id", request.Id).Log()
		return server.UpdateAssessment400JSONResponse{Message: "empty body", RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	assessmentID := request.Id

	// Get assessment to check ownership
	assessment, err := h.assessmentSrv.GetAssessment(ctx, assessmentID)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			logger.Error(err).WithUUID("assessment_id", assessmentID).WithString("step", "get_for_update").Log()
			return server.UpdateAssessment404JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
		default:
			logger.Error(err).WithUUID("assessment_id", assessmentID).WithString("step", "get_for_update").Log()
			return server.UpdateAssessment500JSONResponse{Message: fmt.Sprintf("failed to get assessment: %v", err), RequestId: requestid.FromContextPtr(ctx)}, nil
		}
	}

	logger.Step("assessment_retrieved_for_update").WithString("current_name", assessment.Name).Log()

	if user.Username != assessment.Username || user.Organization != assessment.OrgID {
		message := fmt.Sprintf("forbidden to update assessment %s by user %s", assessmentID, user.Username)
		logger.Error(fmt.Errorf("authorization failed: %s", message)).WithUUID("assessment_id", assessmentID).WithString("username", user.Username).WithString("assessment_username", assessment.Username).Log()
		return server.UpdateAssessment403JSONResponse{Message: message, RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	logger.Step("authorization_check_passed").Log()

	updatedAssessment, err := h.assessmentSrv.UpdateAssessment(ctx, assessmentID, request.Body.Name)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			logger.Error(err).WithUUID("assessment_id", assessmentID).Log()
			return server.UpdateAssessment404JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
		case *service.ErrAgentUpdateForbidden:
			logger.Error(err).WithUUID("assessment_id", assessmentID).Log()
			return server.UpdateAssessment400JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
		default:
			logger.Error(err).WithUUID("assessment_id", assessmentID).Log()
			return server.UpdateAssessment500JSONResponse{Message: fmt.Sprintf("failed to update assessment: %v", err), RequestId: requestid.FromContextPtr(ctx)}, nil
		}
	}

	logger.Success().
		WithString("updated_name", updatedAssessment.Name).
		WithString("source_type", updatedAssessment.SourceType).
		Log()

	apiAssessment, err := mappers.AssessmentToApi(*updatedAssessment)
	if err != nil {
		return server.UpdateAssessment500JSONResponse{Message: fmt.Sprintf("failed to update assessment: %v", err), RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	return server.UpdateAssessment200JSONResponse(apiAssessment), nil
}

// (DELETE /api/v1/assessments/{id})
func (h *ServiceHandler) DeleteAssessment(ctx context.Context, request server.DeleteAssessmentRequestObject) (server.DeleteAssessmentResponseObject, error) {
	logger := log.NewDebugLogger("assessment_handler").
		WithContext(ctx).
		Operation("delete_assessment").
		WithUUID("assessment_id", request.Id).
		Build()

	user := auth.MustHaveUser(ctx)
	logger.Step("extract_user").WithString("org_id", user.Organization).WithString("username", user.Username).Log()

	assessmentID := request.Id

	// Get assessment to check ownership
	assessment, err := h.assessmentSrv.GetAssessment(ctx, assessmentID)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			logger.Error(err).WithUUID("assessment_id", assessmentID).WithString("step", "get_for_delete").Log()
			return server.DeleteAssessment404JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
		default:
			logger.Error(err).WithUUID("assessment_id", assessmentID).WithString("step", "get_for_delete").Log()
			return server.DeleteAssessment500JSONResponse{Message: fmt.Sprintf("failed to get assessment: %v", err), RequestId: requestid.FromContextPtr(ctx)}, nil
		}
	}

	logger.Step("assessment_retrieved_for_delete").WithString("assessment_name", assessment.Name).Log()

	if user.Username != assessment.Username || user.Organization != assessment.OrgID {
		message := fmt.Sprintf("forbidden to delete assessment %s by user with %s", assessmentID, user.Username)
		logger.Error(fmt.Errorf("authorization failed: %s", message)).WithUUID("assessment_id", assessmentID).WithString("username", user.Username).WithString("assessment_username", assessment.Username).Log()
		return server.DeleteAssessment403JSONResponse{Message: message, RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	logger.Step("authorization_check_passed").Log()

	if err := h.assessmentSrv.DeleteAssessment(ctx, assessmentID); err != nil {
		logger.Error(err).WithUUID("assessment_id", assessmentID).Log()
		return server.DeleteAssessment500JSONResponse{Message: fmt.Sprintf("failed to delete assessment: %v", err), RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	logger.Success().WithString("deleted_assessment_name", assessment.Name).Log()
	return server.DeleteAssessment200JSONResponse{}, nil
}

// (DELETE /api/v1/assessments/{id}/job)
func (h *ServiceHandler) CancelAssessmentJob(ctx context.Context, request server.CancelAssessmentJobRequestObject) (server.CancelAssessmentJobResponseObject, error) {
	logger := log.NewDebugLogger("assessment_handler").
		WithContext(ctx).
		Operation("cancel_assessment_job").
		WithUUID("assessment_id", request.Id).
		Build()

	user := auth.MustHaveUser(ctx)

	// Cancel the job and delete snapshot
	err := h.assessmentSrv.CancelJob(ctx, request.Id, user.Organization)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound, *service.ErrNoProcessingJob:
			logger.Error(err).Log()
			return server.CancelAssessmentJob404JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
		case *service.ErrJobCannotBeCancelled:
			logger.Error(err).Log()
			return server.CancelAssessmentJob409JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
		default:
			logger.Error(err).Log()
			return server.CancelAssessmentJob500JSONResponse{Message: fmt.Sprintf("failed to cancel job: %v", err), RequestId: requestid.FromContextPtr(ctx)}, nil
		}
	}

	logger.Success().Log()
	return server.CancelAssessmentJob200Response{}, nil
}

func validateAssessmentData(data interface{}) error {
	v := validator.NewValidator()
	v.Register(validator.NewAssessmentValidationRules()...)
	v.RegisterStructValidation(validator.AssessmentFormValidator(), v1alpha1.AssessmentForm{})
	return v.Struct(data)
}
