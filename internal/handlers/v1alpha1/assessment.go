package v1alpha1

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/handlers/v1alpha1/mappers"
	"github.com/kubev2v/migration-planner/internal/handlers/validator"
	"github.com/kubev2v/migration-planner/internal/service"
	srvMappers "github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

// (GET /api/v1/assessments)
func (h *ServiceHandler) ListAssessments(ctx context.Context, request server.ListAssessmentsRequestObject) (server.ListAssessmentsResponseObject, error) {
	user := auth.MustHaveUser(ctx)

	// first touch the user
	if err := h.authzSrv.CreateUser(ctx, user); err != nil {
		return server.ListAssessments500JSONResponse{Message: fmt.Sprintf("failed to add user and org to spicedb: %v", err)}, nil
	}

	// list assessments for which the user has read permission
	assessmentIds, err := h.authzSrv.ListAssessments(ctx, user)
	if err != nil {
		return server.ListAssessments500JSONResponse{Message: fmt.Sprintf("failed to list permissions: %v", err)}, nil
	}

	filter := service.NewAssessmentFilter().WithArrayIn(assessmentIds)

	assessments, err := h.assessmentSrv.ListAssessments(ctx, filter)
	if err != nil {
		return server.ListAssessments500JSONResponse{Message: fmt.Sprintf("failed to list assessments: %v", err)}, nil
	}

	// Get permissions for all assessments in bulk
	var assessmentIDs []string
	for _, a := range assessments {
		assessmentIDs = append(assessmentIDs, a.ID.String())
	}

	perms, err := h.authzSrv.GetBulkPermissions(ctx, assessmentIDs, user)
	if err != nil {
		zap.S().Warnw("failed to read bulk permissions", "error", err)
		// Continue with empty permissions rather than failing the request
		perms = make(map[string][]model.Permission)
	}

	return server.ListAssessments200JSONResponse(mappers.AssessmentListToApi(assessments, perms)), nil
}

func (h *ServiceHandler) ShareAssessment(ctx context.Context, request server.ShareAssessmentRequestObject) (server.ShareAssessmentResponseObject, error) {
	if request.Body.OrgId == nil && request.Body.UserId == nil {
		return server.ShareAssessment400JSONResponse{Message: "either userId or orgId must be present"}, nil
	}

	user := auth.MustHaveUser(ctx)

	// check if user has the right to share assessment
	hasPermission, err := h.authzSrv.HasPermission(ctx, request.Id.String(), user, model.SharePermission)
	if err != nil {
		return server.ShareAssessment500JSONResponse{Message: fmt.Sprintf("failed to read permissions for assessment %s: %v", request.Id, err)}, nil
	}

	if !hasPermission {
		return server.ShareAssessment403JSONResponse{Message: fmt.Sprintf("user %s does not have sharing permission on assessment %s", user.Username, request.Id)}, nil
	}

	if _, err = h.assessmentSrv.GetAssessment(ctx, request.Id); err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.ShareAssessment404JSONResponse{Message: err.Error()}, nil
		default:
			return server.ShareAssessment500JSONResponse{Message: fmt.Sprintf("failed to get assessment: %v", err)}, nil
		}
	}

	if err := h.authzSrv.WriteRelationship(ctx, request.Id.String(), model.NewUserSubject(*request.Body.UserId), model.ReaderRelationshipKind); err != nil {
		return server.ShareAssessment500JSONResponse{Message: fmt.Sprintf("failed to write share permission on assessment %s for user %s", user.Username, request.Id)}, nil
	}

	return server.ShareAssessment201JSONResponse{}, nil
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

		createForm = mappers.AssessmentFormToCreateForm(form, user)
	}

	// Handle multipart content type (RVTools upload)
	if request.MultipartBody != nil {
		var err error
		createForm, err = mappers.AssessmentCreateFormFromMultipart(request.MultipartBody, user)
		if err != nil {
			return server.CreateAssessment400JSONResponse{Message: fmt.Sprintf("failed to parse multipart form: %v", err)}, nil
		}
	}

	// add permissions
	if err := h.authzSrv.CreateAssessmentRelationship(ctx, createForm.ID.String(), user); err != nil {
		return server.CreateAssessment500JSONResponse{Message: fmt.Sprintf("failed to write permission for the assessment: %v", err)}, nil
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
	user := auth.MustHaveUser(ctx)

	// Check if user has read permission on the assessment
	hasReadPermission, err := h.authzSrv.HasPermission(ctx, assessmentID.String(), user, model.ReadPermission)
	if err != nil {
		return server.GetAssessment500JSONResponse{Message: fmt.Sprintf("failed to check permissions for assessment %s: %v", assessmentID, err)}, nil
	}

	if !hasReadPermission {
		return server.GetAssessment403JSONResponse{Message: fmt.Sprintf("user %s does not have read permission on assessment %s", user.Username, assessmentID)}, nil
	}

	assessment, err := h.assessmentSrv.GetAssessment(ctx, assessmentID)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.GetAssessment404JSONResponse{Message: err.Error()}, nil
		default:
			return server.GetAssessment500JSONResponse{Message: fmt.Sprintf("failed to get assessment: %v", err)}, nil
		}
	}

	return server.GetAssessment200JSONResponse(mappers.AssessmentToApi(*assessment)), nil
}

// (PUT /api/v1/assessments/{id})
func (h *ServiceHandler) UpdateAssessment(ctx context.Context, request server.UpdateAssessmentRequestObject) (server.UpdateAssessmentResponseObject, error) {
	if request.Body == nil {
		return server.UpdateAssessment400JSONResponse{Message: "empty body"}, nil
	}

	assessmentID := request.Id
	user := auth.MustHaveUser(ctx)

	// Check if user has edit permission on the assessment
	hasEditPermission, err := h.authzSrv.HasPermission(ctx, assessmentID.String(), user, model.EditPermission)
	if err != nil {
		return server.UpdateAssessment500JSONResponse{Message: fmt.Sprintf("failed to check permissions for assessment %s: %v", assessmentID, err)}, nil
	}

	if !hasEditPermission {
		return server.UpdateAssessment403JSONResponse{Message: fmt.Sprintf("user %s does not have edit permission on assessment %s", user.Username, assessmentID)}, nil
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

func (h *ServiceHandler) UnshareAssessment(ctx context.Context, request server.UnshareAssessmentRequestObject) (server.UnshareAssessmentResponseObject, error) {
	return nil, nil
}

// (DELETE /api/v1/assessments/{id})
func (h *ServiceHandler) DeleteAssessment(ctx context.Context, request server.DeleteAssessmentRequestObject) (server.DeleteAssessmentResponseObject, error) {
	assessmentID := request.Id
	user := auth.MustHaveUser(ctx)

	// Get assessment to check if it exists
	_, err := h.assessmentSrv.GetAssessment(ctx, assessmentID)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.DeleteAssessment404JSONResponse{Message: err.Error()}, nil
		default:
			return server.DeleteAssessment500JSONResponse{Message: fmt.Sprintf("failed to get assessment: %v", err)}, nil
		}
	}

	// Check if user has delete permission on the assessment
	hasDeletePermission, err := h.authzSrv.HasPermission(ctx, assessmentID.String(), user, model.DeletePermission)
	if err != nil {
		return server.DeleteAssessment500JSONResponse{Message: fmt.Sprintf("failed to check permissions for assessment %s: %v", assessmentID, err)}, nil
	}

	if !hasDeletePermission {
		return server.DeleteAssessment403JSONResponse{Message: fmt.Sprintf("user %s does not have delete permission on assessment %s", user.Username, assessmentID)}, nil
	}

	if err := h.assessmentSrv.DeleteAssessment(ctx, assessmentID); err != nil {
		return server.DeleteAssessment500JSONResponse{Message: fmt.Sprintf("failed to delete assessment: %v", err)}, nil
	}

	if err := h.authzSrv.DeleteAssessmentAllRelationships(ctx, assessmentID.String()); err != nil {
		zap.S().Warnw("failed to delete relationships for assessment", "id", assessmentID.String(), "error", err) // don't really care if we succed or not
	}

	return server.DeleteAssessment200JSONResponse{}, nil
}

func validateAssessmentData(data interface{}) error {
	v := validator.NewValidator()
	v.Register(validator.NewAssessmentValidationRules()...)
	v.RegisterStructValidation(validator.AssessmentFormValidator(), v1alpha1.AssessmentForm{})
	return v.Struct(data)
}
