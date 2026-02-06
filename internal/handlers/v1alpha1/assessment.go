package v1alpha1

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/handlers/v1alpha1/mappers"
	"github.com/kubev2v/migration-planner/internal/handlers/validator"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/pkg/inventory/converters"
	"github.com/kubev2v/migration-planner/pkg/log"
	"github.com/kubev2v/migration-planner/pkg/requestid"
)

// OrderedAssessmentListResponse wraps AssessmentList to ensure inventories
// are marshaled with ordered clusters
type OrderedAssessmentListResponse struct {
	assessments v1alpha1.AssessmentList
}

// GetAssessments returns the underlying assessments list for testing purposes
func (r *OrderedAssessmentListResponse) GetAssessments() v1alpha1.AssessmentList {
	return r.assessments
}

// VisitListAssessmentsResponse implements server.ListAssessmentsResponseObject
// to ensure clusters are ordered when marshaling the response
func (r *OrderedAssessmentListResponse) VisitListAssessmentsResponse(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)

	// Create a response structure with OrderedInventory for each snapshot
	type orderedSnapshot struct {
		CreatedAt time.Time                    `json:"createdAt"`
		Inventory *converters.OrderedInventory `json:"inventory"`
	}

	type assessmentWithOrderedInventory struct {
		Id             string                        `json:"id"`
		Name           string                        `json:"name"`
		OwnerFirstName *string                       `json:"ownerFirstName,omitempty"`
		OwnerLastName  *string                       `json:"ownerLastName,omitempty"`
		SourceType     v1alpha1.AssessmentSourceType `json:"sourceType"`
		SourceId       *openapi_types.UUID           `json:"sourceId,omitempty"`
		CreatedAt      time.Time                     `json:"createdAt"`
		Snapshots      []orderedSnapshot             `json:"snapshots"`
	}

	orderedAssessments := make([]assessmentWithOrderedInventory, len(r.assessments))
	for i, assessment := range r.assessments {
		orderedAssessments[i] = assessmentWithOrderedInventory{
			Id:             assessment.Id.String(),
			Name:           assessment.Name,
			OwnerFirstName: assessment.OwnerFirstName,
			OwnerLastName:  assessment.OwnerLastName,
			SourceType:     assessment.SourceType,
			SourceId:       assessment.SourceId,
			CreatedAt:      assessment.CreatedAt,
			Snapshots:      make([]orderedSnapshot, len(assessment.Snapshots)),
		}
		for j, snapshot := range assessment.Snapshots {
			inv := snapshot.Inventory
			orderedAssessments[i].Snapshots[j] = orderedSnapshot{
				CreatedAt: snapshot.CreatedAt,
				Inventory: &converters.OrderedInventory{Inventory: &inv},
			}
		}
	}

	return json.NewEncoder(w).Encode(orderedAssessments)
}

// (GET /api/v1/assessments)
func (h *ServiceHandler) ListAssessments(ctx context.Context, request server.ListAssessmentsRequestObject) (server.ListAssessmentsResponseObject, error) {
	logger := log.NewDebugLogger("assessment_handler").
		WithContext(ctx).
		Operation("list_assessments").
		Build()

	user := auth.MustHaveUser(ctx)
	logger.Step("extract_user").WithString("org_id", user.Organization).WithString("username", user.Username).Log()

	filter := service.NewAssessmentFilter(user.Username, user.Organization)

	// Extract sourceId from query parameter if provided
	if request.Params.SourceId != nil {
		sourceIdStr := request.Params.SourceId.String()
		filter = filter.WithSourceID(sourceIdStr)
		logger.Step("filter_by_source_id").WithString("source_id", sourceIdStr).Log()
	}

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

	return &OrderedAssessmentListResponse{assessments: apiAssessments}, nil
}

// (POST /api/v1/assessments)
func (h *ServiceHandler) CreateAssessment(ctx context.Context, request server.CreateAssessmentRequestObject) (server.CreateAssessmentResponseObject, error) {
	logger := log.NewDebugLogger("assessment_handler").
		WithContext(ctx).
		Operation("create_assessment").
		WithRequestBody("request_body", request.Body).
		Build()

	user := auth.MustHaveUser(ctx)
	logger.Step("extract_user").WithString("org_id", user.Organization).WithString("username", user.Username).Log()

	if request.Body == nil {
		logger.Error(fmt.Errorf("empty request body")).Log()
		return server.CreateAssessment400JSONResponse{Message: "empty body", RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	form := v1alpha1.AssessmentForm(*request.Body)
	if err := validateAssessmentData(form); err != nil {
		logger.Error(err).WithString("step", "validation").Log()
		return server.CreateAssessment400JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	createForm := mappers.AssessmentFormToCreateForm(form, user)
	logger.Step("mapped_form").WithString("source_type", createForm.Source).Log()

	logger.Step("create_assessment").
		WithUUID("id", createForm.ID).
		WithString("name", createForm.Name).
		WithString("org_id", createForm.OrgID).
		WithString("source", createForm.Source).
		WithUUIDPtr("source_id", createForm.SourceID).
		Log()

	assessment, err := h.assessmentSrv.CreateAssessment(ctx, createForm)
	if err != nil {
		switch err.(type) {
		case *service.ErrAssessmentCreationForbidden:
			logger.Error(err).WithString("step", "authorization").Log()
			return server.CreateAssessment401JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
		case *service.ErrSourceHasNoInventory:
			logger.Error(err).WithString("step", "inventory_check").Log()
			return server.CreateAssessment400JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
		case *service.ErrInventoryHasNoVMs:
			logger.Error(err).WithString("step", "inventory_validation").Log()
			return server.CreateAssessment400JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
		case *service.ErrDuplicateKey:
			logger.Error(err).WithString("step", "validate_input").Log()
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

func validateAssessmentData(data interface{}) error {
	v := validator.NewValidator()
	v.Register(validator.NewAssessmentValidationRules()...)
	v.RegisterStructValidation(validator.AssessmentFormValidator(), v1alpha1.AssessmentForm{})
	return v.Struct(data)
}
