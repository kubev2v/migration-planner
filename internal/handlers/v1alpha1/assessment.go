package v1alpha1

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"go.uber.org/zap"

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

	// first touch the user
	if err := h.authzSrv.CreateUser(ctx, user); err != nil {
		return server.ListAssessments500JSONResponse{Message: fmt.Sprintf("failed to add user and org to spicedb: %v", err)}, nil
	}

	// list assessments for which the user has read permission
	assessmentIds, err := h.authzSrv.ListAssessments(ctx, user)
	if err != nil {
		return server.ListAssessments500JSONResponse{Message: fmt.Sprintf("failed to list permissions: %v", err)}, nil
	}

	if len(assessmentIds) == 0 {
		return server.ListAssessments200JSONResponse{}, nil
	}

	filter := service.NewAssessmentFilter().WithArrayIn(assessmentIds)

	assessments, err := h.assessmentSrv.ListAssessments(ctx, filter)
	if err != nil {
		logger.Error(err).Log()
		return server.ListAssessments500JSONResponse{Message: fmt.Sprintf("failed to list assessments: %v", err), RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	logger.Success().WithInt("count", len(assessments)).Log()

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
	}

	logger.Step("create_assessment").
		WithUUID("id", createForm.ID).
		WithString("name", createForm.Name).
		WithString("org_id", createForm.OrgID).
		WithString("source", createForm.Source).
		WithUUIDPtr("source_id", createForm.SourceID).
		Log()

	// add permissions
	if err := h.authzSrv.CreateAssessmentRelationship(ctx, createForm.ID.String(), user); err != nil {
		return server.CreateAssessment500JSONResponse{Message: fmt.Sprintf("failed to write permission for the assessment: %v", err)}, nil
	}

	assessment, err := h.assessmentSrv.CreateAssessment(ctx, createForm)
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
			return server.CreateAssessment400JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
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

	return server.CreateAssessment201JSONResponse(mappers.AssessmentToApi(*assessment, []model.Permission{model.ReadPermission, model.EditPermission, model.SharePermission, model.DeletePermission})), nil
}

// (GET /api/v1/assessments/{id})
func (h *ServiceHandler) GetAssessment(ctx context.Context, request server.GetAssessmentRequestObject) (server.GetAssessmentResponseObject, error) {
	logger := log.NewDebugLogger("assessment_handler").
		WithContext(ctx).
		Operation("get_assessment").
		WithUUID("assessment_id", request.Id).
		Build()

	assessmentID := request.Id
	user := auth.MustHaveUser(ctx)

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
	// Check if user has read permission on the assessment
	permissions, err := h.authzSrv.GetPermissions(ctx, assessmentID.String(), user)
	if err != nil {
		logger.Error(err).WithUUID("assessment_id", assessmentID).Log()
		return server.GetAssessment500JSONResponse{Message: fmt.Sprintf("failed to check permissions for assessment %s: %v", assessmentID, err)}, nil
	}

	logger.Step("permissions_retrieved").WithString("assessment_name", assessment.Name).Log()

	if !slices.Contains(permissions, model.ReadPermission) {
		return server.GetAssessment403JSONResponse{Message: fmt.Sprintf("user %s does not have read permission on assessment %s", user.Username, assessmentID)}, nil
	}

	logger.Step("authorization_check_passed").Log()
	logger.Success().WithString("source_type", assessment.SourceType).Log()

	return server.GetAssessment200JSONResponse(mappers.AssessmentToApi(*assessment, permissions)), nil
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

	// check if assessment exists before checking for permissions
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

	// Check if user has edit permission on the assessment
	permissions, err := h.authzSrv.GetPermissions(ctx, assessmentID.String(), user)
	if err != nil {
		logger.Error(err).WithUUID("assessment_id", assessmentID).Log()
		return server.UpdateAssessment500JSONResponse{Message: fmt.Sprintf("failed to check permissions for assessment %s: %v", assessmentID, err)}, nil
	}

	if !slices.Contains(permissions, model.EditPermission) {
		return server.UpdateAssessment403JSONResponse{Message: fmt.Sprintf("user %s does not have edit permission on assessment %s", user.Username, assessmentID)}, nil
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

	return server.UpdateAssessment200JSONResponse(mappers.AssessmentToApi(*updatedAssessment, permissions)), nil
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

	// Get assessment to check if it exists
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

	// Check if user has delete permission on the assessment
	hasDeletePermission, err := h.authzSrv.HasPermission(ctx, assessmentID.String(), user, model.DeletePermission)
	if err != nil {
		return server.DeleteAssessment500JSONResponse{Message: fmt.Sprintf("failed to check permissions for assessment %s: %v", assessmentID, err)}, nil
	}

	if !hasDeletePermission {
		return server.DeleteAssessment403JSONResponse{Message: fmt.Sprintf("user %s does not have delete permission on assessment %s", user.Username, assessmentID)}, nil
	}

	logger.Step("authorization_check_passed").Log()

	if err := h.assessmentSrv.DeleteAssessment(ctx, assessmentID); err != nil {
		logger.Error(err).WithUUID("assessment_id", assessmentID).Log()
		return server.DeleteAssessment500JSONResponse{Message: fmt.Sprintf("failed to delete assessment: %v", err), RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	logger.Success().WithString("deleted_assessment_name", assessment.Name).Log()
	if err := h.authzSrv.DeleteAllRelationships(ctx, assessmentID.String()); err != nil {
		zap.S().Warnw("failed to delete relationships for assessment", "id", assessmentID.String(), "error", err) // don't really care if we succed or not
	}

	return server.DeleteAssessment200JSONResponse{}, nil
}

// (GET /api/v1/assessments/{id}/relationships)
func (h *ServiceHandler) ListAssessmentRelationships(ctx context.Context, request server.ListAssessmentRelationshipsRequestObject) (server.ListAssessmentRelationshipsResponseObject, error) {
	logger := log.NewDebugLogger("assessment_handler").
		WithContext(ctx).
		Operation("list_assessment_relationships").
		WithUUID("assessment_id", request.Id).
		Build()

	user := auth.MustHaveUser(ctx)
	logger.Step("extract_user").WithString("org_id", user.Organization).WithString("username", user.Username).Log()

	// Check if assessment exists
	if _, err := h.assessmentSrv.GetAssessment(ctx, request.Id); err != nil {
		logger.Error(err).WithString("step", "get_assessment").Log()
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.ListAssessmentRelationships404JSONResponse{Message: err.Error()}, nil
		default:
			return server.ListAssessmentRelationships500JSONResponse{Message: fmt.Sprintf("failed to get assessment: %v", err)}, nil
		}
	}

	// Check if user has read permission on the assessment
	hasPermission, err := h.authzSrv.HasPermission(ctx, request.Id.String(), user, model.ReadPermission)
	if err != nil {
		logger.Error(err).WithString("step", "check_permission").Log()
		return server.ListAssessmentRelationships500JSONResponse{Message: fmt.Sprintf("failed to read permissions for assessment %s: %v", request.Id, err)}, nil
	}

	if !hasPermission {
		logger.Error(errors.New("no_read_permission")).WithUUID("assessment_id", request.Id).WithString("user", user.Username).Log()
		return server.ListAssessmentRelationships403JSONResponse{Message: fmt.Sprintf("user %s does not have read permission on assessment %s", user.Username, request.Id)}, nil
	}

	relationships, err := h.authzSrv.ListRelationships(ctx, request.Id.String())
	if err != nil {
		logger.Error(err).WithString("step", "list_relationships").Log()
		return server.ListAssessmentRelationships500JSONResponse{Message: fmt.Sprintf("failed to list relationships for assessment %s: %v", request.Id, err)}, nil
	}

	// Map relationships to API format
	apiRelationships := mappers.RelationshipListToApi(relationships)

	logger.Success().
		WithUUID("assessment_id", request.Id).
		WithString("user", user.Username).
		WithInt("relationship_count", len(apiRelationships)).
		Log()

	return server.ListAssessmentRelationships200JSONResponse(apiRelationships), nil
}

// (POST /api/v1/assessments/{id}/relationships)
func (h *ServiceHandler) AddAssessmentRelationship(ctx context.Context, request server.AddAssessmentRelationshipRequestObject) (server.AddAssessmentRelationshipResponseObject, error) {
	logger := log.NewDebugLogger("assessment_handler").
		WithContext(ctx).
		Operation("add_assessment_relationship").
		WithInt("relationships_count", len(request.Body.Relationships)).
		Build()

	user := auth.MustHaveUser(ctx)

	if _, err := h.assessmentSrv.GetAssessment(ctx, request.Id); err != nil {
		logger.Error(err).WithString("step", "get_assessment").Log()
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.AddAssessmentRelationship404JSONResponse{Message: err.Error()}, nil
		default:
			return server.AddAssessmentRelationship500JSONResponse{Message: fmt.Sprintf("failed to get assessment: %v", err)}, nil
		}
	}

	// check if user has the right to share assessment
	hasPermission, err := h.authzSrv.HasPermission(ctx, request.Id.String(), user, model.SharePermission)
	if err != nil {
		logger.Error(err).WithString("step", "check_permission").Log()
		return server.AddAssessmentRelationship500JSONResponse{Message: fmt.Sprintf("failed to read permissions for assessment %s: %v", request.Id, err)}, nil
	}

	if !hasPermission {
		logger.Error(errors.New("no_share_permission")).WithUUID("assessment_id", request.Id).WithString("user", user.Username).Log()
		return server.AddAssessmentRelationship403JSONResponse{Message: fmt.Sprintf("user %s does not have sharing permission on assessment %s", user.Username, request.Id)}, nil
	}

	relationships := make([]model.Relationship, 0, len(request.Body.Relationships))
	for _, rel := range request.Body.Relationships {
		var relationshipKind model.RelationshipKind
		switch rel.Relationship {
		case v1alpha1.Editor:
			relationshipKind = model.EditorRelationshipKind
		case v1alpha1.Viewer:
			relationshipKind = model.ViewerRelationshipKind
		default:
			return server.AddAssessmentRelationship400JSONResponse{Message: "unsupported relationship"}, nil
		}
		relationships = append(relationships, model.NewRelationship(request.Id.String(), model.NewUserSubject(rel.UserId), relationshipKind))
		logger.Step("relationship_queued").
			WithString("user_id", rel.UserId).
			WithString("relationship", string(rel.Relationship)).
			Log()
	}

	if err := h.authzSrv.WriteRelationships(ctx, relationships...); err != nil {
		logger.Error(err).WithString("step", "write_permissions").Log()
		return server.AddAssessmentRelationship500JSONResponse{Message: fmt.Sprintf("failed to write share permissions on assessment %s: %v", request.Id, err)}, nil
	}

	logger.Success().
		WithUUID("assessment_id", request.Id).
		WithString("user", user.Username).
		WithInt("relationships_added", len(request.Body.Relationships)).
		Log()

	return server.AddAssessmentRelationship201JSONResponse{}, nil
}

func (h *ServiceHandler) RemoveAssessmentRelationship(ctx context.Context, request server.RemoveAssessmentRelationshipRequestObject) (server.RemoveAssessmentRelationshipResponseObject, error) {
	logger := log.NewDebugLogger("assessment_handler").
		WithContext(ctx).
		Operation("remove_assessment_relationship").
		WithInt("relationships_count", len(request.Body.Relationships)).
		Build()

	user := auth.MustHaveUser(ctx)

	if _, err := h.assessmentSrv.GetAssessment(ctx, request.Id); err != nil {
		logger.Error(err).WithString("step", "get_assessment").Log()
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.RemoveAssessmentRelationship404JSONResponse{Message: err.Error()}, nil
		default:
			return server.RemoveAssessmentRelationship500JSONResponse{Message: fmt.Sprintf("failed to get assessment: %v", err)}, nil
		}
	}

	// check if user has the right to share assessment
	hasPermission, err := h.authzSrv.HasPermission(ctx, request.Id.String(), user, model.SharePermission)
	if err != nil {
		logger.Error(err).WithString("step", "check_permission").Log()
		return server.RemoveAssessmentRelationship500JSONResponse{Message: fmt.Sprintf("failed to read permissions for assessment %s: %v", request.Id, err)}, nil
	}

	if !hasPermission {
		logger.Error(errors.New("no_share_permission")).WithUUID("assessment_id", request.Id).WithString("user", user.Username).Log()
		return server.RemoveAssessmentRelationship403JSONResponse{Message: fmt.Sprintf("user %s does not have sharing permission on assessment %s", user.Username, request.Id)}, nil
	}

	relationships := make([]model.Relationship, 0, len(request.Body.Relationships))
	for _, rel := range request.Body.Relationships {
		var relationshipKind model.RelationshipKind
		switch rel.Relationship {
		case v1alpha1.Editor:
			relationshipKind = model.EditorRelationshipKind
		case v1alpha1.Viewer:
			relationshipKind = model.ViewerRelationshipKind
		default:
			return server.RemoveAssessmentRelationship400JSONResponse{Message: "unsupported relationship"}, nil
		}
		relationships = append(relationships, model.NewRelationship(request.Id.String(), model.NewUserSubject(rel.UserId), relationshipKind))
		logger.Step("relationship_queued").
			WithString("user_id", rel.UserId).
			WithString("relationship", string(rel.Relationship)).
			Log()
	}

	if err := h.authzSrv.DeleteRelationships(ctx, relationships...); err != nil {
		logger.Error(err).WithString("step", "delete_permissions").Log()
		return server.RemoveAssessmentRelationship500JSONResponse{Message: fmt.Sprintf("failed to remove share permissions on assessment %s: %v", request.Id, err)}, nil
	}

	logger.Success().
		WithUUID("assessment_id", request.Id).
		WithString("user", user.Username).
		WithInt("relationships_removed", len(request.Body.Relationships)).
		Log()

	return server.RemoveAssessmentRelationship200JSONResponse{}, nil
}

// (POST /api/v1/assessments/{id}/relationships/organization)
func (h *ServiceHandler) ShareAssessmentWithOrganization(ctx context.Context, request server.ShareAssessmentWithOrganizationRequestObject) (server.ShareAssessmentWithOrganizationResponseObject, error) {
	logger := log.NewDebugLogger("assessment_handler").
		WithContext(ctx).
		Operation("share_assessment_with_organization").
		WithUUID("assessment_id", request.Id).
		Build()

	user := auth.MustHaveUser(ctx)
	logger.Step("extract_user").WithString("org_id", user.Organization).WithString("username", user.Username).Log()

	// Get assessment to check if it exists and verify ownership
	assessment, err := h.assessmentSrv.GetAssessment(ctx, request.Id)
	if err != nil {
		logger.Error(err).WithString("step", "get_assessment").Log()
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.ShareAssessmentWithOrganization404JSONResponse{Message: err.Error()}, nil
		default:
			return server.ShareAssessmentWithOrganization500JSONResponse{Message: fmt.Sprintf("failed to get assessment: %v", err)}, nil
		}
	}

	// Check if user is the owner (only owners can share with organization)
	if assessment.Username != user.Username {
		logger.Error(errors.New("not_owner")).WithUUID("assessment_id", request.Id).WithString("user", user.Username).WithString("owner", assessment.Username).Log()
		return server.ShareAssessmentWithOrganization401JSONResponse{Message: fmt.Sprintf("user %s is not the owner of assessment %s", user.Username, request.Id)}, nil
	}

	// Share with organization
	relationship := model.NewRelationship(request.Id.String(), model.NewOrganizationSubject(user.Organization), model.OrganizationRelationshipKind)

	if err := h.authzSrv.WriteRelationships(ctx, relationship); err != nil {
		logger.Error(err).WithString("step", "write_organization_relationship").Log()
		return server.ShareAssessmentWithOrganization500JSONResponse{Message: fmt.Sprintf("failed to share assessment %s with organization: %v", request.Id, err)}, nil
	}

	logger.Success().
		WithUUID("assessment_id", request.Id).
		WithString("user", user.Username).
		WithString("organization", user.Organization).
		Log()

	return server.ShareAssessmentWithOrganization201JSONResponse{}, nil
}

// (DELETE /api/v1/assessments/{id}/relationships/organization)
func (h *ServiceHandler) UnshareAssessmentWithOrganization(ctx context.Context, request server.UnshareAssessmentWithOrganizationRequestObject) (server.UnshareAssessmentWithOrganizationResponseObject, error) {
	logger := log.NewDebugLogger("assessment_handler").
		WithContext(ctx).
		Operation("unshare_assessment_with_organization").
		WithUUID("assessment_id", request.Id).
		Build()

	user := auth.MustHaveUser(ctx)
	logger.Step("extract_user").WithString("org_id", user.Organization).WithString("username", user.Username).Log()

	// Get assessment to check if it exists and verify ownership
	assessment, err := h.assessmentSrv.GetAssessment(ctx, request.Id)
	if err != nil {
		logger.Error(err).WithString("step", "get_assessment").Log()
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.UnshareAssessmentWithOrganization404JSONResponse{Message: err.Error()}, nil
		default:
			return server.UnshareAssessmentWithOrganization500JSONResponse{Message: fmt.Sprintf("failed to get assessment: %v", err)}, nil
		}
	}

	// Check if user is the owner (only owners can unshare from organization)
	if assessment.Username != user.Username {
		logger.Error(errors.New("not_owner")).WithUUID("assessment_id", request.Id).WithString("user", user.Username).WithString("owner", assessment.Username).Log()
		return server.UnshareAssessmentWithOrganization401JSONResponse{Message: fmt.Sprintf("user %s is not the owner of assessment %s", user.Username, request.Id)}, nil
	}

	// Unshare from organization
	if err := h.authzSrv.DeleteRelationships(ctx,
		model.NewRelationship(request.Id.String(), model.NewOrganizationSubject(user.Organization), model.OrganizationRelationshipKind),
	); err != nil {
		logger.Error(err).WithString("step", "delete_organization_relationship").Log()
		return server.UnshareAssessmentWithOrganization500JSONResponse{Message: fmt.Sprintf("failed to unshare assessment %s from organization: %v", request.Id, err)}, nil
	}

	logger.Success().
		WithUUID("assessment_id", request.Id).
		WithString("user", user.Username).
		WithString("organization", user.Organization).
		Log()

	return server.UnshareAssessmentWithOrganization200JSONResponse{}, nil
}

func validateAssessmentData(data interface{}) error {
	v := validator.NewValidator()
	v.Register(validator.NewAssessmentValidationRules()...)
	v.RegisterStructValidation(validator.AssessmentFormValidator(), v1alpha1.AssessmentForm{})
	return v.Struct(data)
}
