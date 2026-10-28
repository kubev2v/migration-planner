package v1alpha1

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/handlers/v1alpha1/mappers"
	"github.com/kubev2v/migration-planner/internal/handlers/validator"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/log"
)

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
		return server.CreateAssessment400JSONResponse{Message: "empty body"}, nil
	}

	form := *request.Body

	// Validate name first for better error message
	if err := validator.ValidateName(form.Name); err != nil {
		logger.Error(err).WithString("step", "validation").Log()
		return server.CreateAssessment400JSONResponse{Message: err.Error()}, nil
	}

	if err := validateAssessmentData(form); err != nil {
		logger.Error(err).WithString("step", "validation").Log()
		return server.CreateAssessment400JSONResponse{Message: err.Error()}, nil
	}

	createForm := mappers.AssessmentFormToCreateForm(form, user)
	logger.Step("mapped_form").WithString("source_type", createForm.SourceType).Log()

	logger.Step("create_assessment").
		WithUUID("id", createForm.ID).
		WithString("name", createForm.Name).
		WithString("org_id", createForm.OrgID).
		WithString("source", createForm.SourceType).
		WithUUIDPtr("source_id", createForm.SourceID).
		Log()

	assessment, err := h.assessmentSrv.CreateAssessment(ctx, createForm)
	if err != nil {
		switch err.(type) {
		case *service.ErrAssessmentCreationForbidden:
			logger.Error(err).WithString("step", "authorization").Log()
			return server.CreateAssessment401JSONResponse{Message: err.Error()}, nil
		case *service.ErrSourceHasNoInventory:
			logger.Error(err).WithString("step", "inventory_check").Log()
			return server.CreateAssessment400JSONResponse{Message: err.Error()}, nil
		case *service.ErrInventoryHasNoVMs:
			logger.Error(err).WithString("step", "inventory_validation").Log()
			return server.CreateAssessment400JSONResponse{Message: err.Error()}, nil
		case *service.ErrDuplicateKey:
			logger.Error(err).WithString("step", "validate_input").Log()
			return server.CreateAssessment400JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).Log()
			return server.CreateAssessment500JSONResponse{Message: err.Error()}, nil
		}
	}

	logger.Success().
		WithUUID("assessment_id", assessment.ID).
		WithString("assessment_name", assessment.Name).
		WithString("source_type", assessment.SourceType).
		Log()

	return server.CreateAssessment201JSONResponse(mappers.AssessmentToV2API(assessment)), nil
}

// CreateRVToolsAssessmentV2 (POST /api/v2/assessments/rvtools)
func (h *ServiceHandler) CreateRVToolsAssessmentV2(ctx context.Context, request server.CreateRVToolsAssessmentV2RequestObject) (server.CreateRVToolsAssessmentV2ResponseObject, error) {
	logger := log.NewDebugLogger("job_handler").
		WithContext(ctx).
		Operation("create_rvtools_assessment").
		Build()

	user := auth.MustHaveUser(ctx)
	logger.Step("extract_user").WithString("org_id", user.Organization).WithString("username", user.Username).Log()

	if request.Body == nil {
		logger.Error(fmt.Errorf("empty request body")).Log()
		return server.CreateRVToolsAssessmentV2400JSONResponse{Message: "empty body"}, nil
	}

	var name string
	var fileSize int64

	canCleanup := true
	tmpFile, err := os.CreateTemp("", "rvtools-upload-*.xlsx")
	if err != nil {
		logger.Error(err).WithString("step", "create_temp_file").Log()
		return server.CreateRVToolsAssessmentV2500JSONResponse{Message: "failed to create temp file"}, nil
	}
	defer func() {
		if canCleanup {
			_ = tmpFile.Close()
			_ = os.Remove(tmpFile.Name())
		}
	}()

	for {
		part, err := request.Body.NextPart()
		if err != nil {
			if err == io.EOF {
				break
			}
			logger.Error(err).WithString("step", "parse_multipart").Log()
			return server.CreateRVToolsAssessmentV2400JSONResponse{Message: "failed to parse form"}, nil
		}

		switch part.FormName() {
		case "name":
			nameBytes, err := io.ReadAll(io.LimitReader(part, 1024))
			_ = part.Close()
			if err != nil {
				logger.Error(err).WithString("step", "read_name").Log()
				return server.CreateRVToolsAssessmentV2400JSONResponse{Message: "failed to read name"}, nil
			}
			name = string(nameBytes)
		case "file":
			n, copyErr := io.Copy(tmpFile, io.LimitReader(part, maxUploadSize+1))
			closeErr := tmpFile.Close()
			_ = part.Close()

			if copyErr != nil {
				logger.Error(copyErr).WithString("step", "write_temp_file").Log()
				return server.CreateRVToolsAssessmentV2400JSONResponse{Message: "failed to read file"}, nil
			}

			if closeErr != nil {
				logger.Error(closeErr).WithString("step", "close_temp_file").Log()
				return server.CreateRVToolsAssessmentV2500JSONResponse{Message: "failed to write temp file"}, nil
			}

			if n == 0 {
				logger.Error(fmt.Errorf("rvtools file is empty")).WithString("step", "validation").Log()
				return server.CreateRVToolsAssessmentV2400JSONResponse{Message: "rvtools file is empty"}, nil
			}

			if n > maxUploadSize {
				logger.Error(fmt.Errorf("file exceeds maximum upload size")).WithString("step", "validation").Log()
				return server.CreateRVToolsAssessmentV2400JSONResponse{Message: fmt.Sprintf("file exceeds maximum upload size of %d MiB", maxUploadSize>>20)}, nil
			}
			fileSize = n
		default:
			_ = part.Close()
		}
	}

	if err := validator.ValidateName(name); err != nil {
		logger.Error(err).WithString("step", "validation").Log()
		return server.CreateRVToolsAssessmentV2400JSONResponse{Message: err.Error()}, nil
	}

	if err := validateXLSXFile(tmpFile.Name()); err != nil {
		logger.Error(err).WithString("step", "validation").Log()
		var invalid *validator.ErrInvalidFile
		if errors.As(err, &invalid) {
			return server.CreateRVToolsAssessmentV2400JSONResponse{Message: invalid.Error()}, nil
		}
		return server.CreateRVToolsAssessmentV2500JSONResponse{Message: "failed to read file"}, nil
	}

	logger.Step("file_received").WithInt("file_size", int(fileSize)).Log()

	jobArgs := model.RVToolsJobArgs{
		Name:          name,
		OperationType: model.CreateOperationType,
		File:          tmpFile.Name(),
		OrgID:         user.Organization,
		Username:      user.Username,
		FirstName:     user.FirstName,
		LastName:      user.LastName,
	}

	job, err := h.jobSrv.CreateRVToolsJob(ctx, jobArgs)
	if err != nil {
		logger.Error(err).Log()
		return server.CreateRVToolsAssessmentV2500JSONResponse{Message: "failed to create job"}, nil
	}
	canCleanup = false

	logger.Success().WithParam("job_id", job.Id).Log()

	return server.CreateRVToolsAssessmentV2202JSONResponse(*job), nil
}

// UploadRvtoolsToAssessment (PUT /api/v2/assessments/{id}/rvtools)
func (h *ServiceHandler) UploadRvtoolsToAssessment(ctx context.Context, request server.UploadRvtoolsToAssessmentRequestObject) (server.UploadRvtoolsToAssessmentResponseObject, error) {
	logger := log.NewDebugLogger("job_handler").
		WithContext(ctx).
		Operation("update_rvtools_assessment").
		Build()

	user := auth.MustHaveUser(ctx)
	logger.Step("extract_user").WithString("org_id", user.Organization).WithString("username", user.Username).Log()

	_, err := h.assessmentSrv.GetAssessment(ctx, request.Id)
	if err != nil {
		logger.Error(err).Log()
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.UploadRvtoolsToAssessment404JSONResponse{Message: err.Error()}, nil
		case *service.ErrForbidden:
			return server.UploadRvtoolsToAssessment403JSONResponse{Message: err.Error()}, nil
		default:
			return server.UploadRvtoolsToAssessment500JSONResponse{Message: "failed to get assessment"}, nil
		}
	}

	if request.Body == nil {
		logger.Error(fmt.Errorf("empty request body")).Log()
		return server.UploadRvtoolsToAssessment400JSONResponse{Message: "empty body"}, nil
	}

	part, err := request.Body.NextPart()
	if err != nil {
		logger.Error(err).WithString("step", "parse_multipart").Log()
		return server.UploadRvtoolsToAssessment400JSONResponse{Message: "failed to parse form"}, nil
	}

	if part.FormName() != "file" {
		return server.UploadRvtoolsToAssessment400JSONResponse{Message: "file is required"}, nil
	}

	tmpFile, err := os.CreateTemp("", "rvtools-upload-*.xlsx")
	if err != nil {
		_ = part.Close()
		logger.Error(err).WithString("step", "create_temp_file").Log()
		return server.UploadRvtoolsToAssessment500JSONResponse{Message: "failed to create temp file"}, nil
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpFile.Name())
		}
	}()

	n, copyErr := io.Copy(tmpFile, io.LimitReader(part, maxUploadSize+1))
	closeErr := tmpFile.Close()
	partCloseErr := part.Close()
	if copyErr != nil {
		logger.Error(copyErr).WithString("step", "write_temp_file").Log()
		return server.UploadRvtoolsToAssessment400JSONResponse{Message: "failed to read file"}, nil
	}

	if closeErr != nil {
		logger.Error(closeErr).WithString("step", "close_temp_file").Log()
		return server.UploadRvtoolsToAssessment500JSONResponse{Message: "failed to write temp file"}, nil
	}

	if partCloseErr != nil {
		logger.Error(partCloseErr).WithString("step", "close_part").Log()
		return server.UploadRvtoolsToAssessment500JSONResponse{Message: "failed to close part"}, nil
	}

	if n == 0 {
		logger.Error(fmt.Errorf("rvtools file is empty")).WithString("step", "validation").Log()
		return server.UploadRvtoolsToAssessment400JSONResponse{Message: "rvtools file is empty"}, nil
	}

	if n > maxUploadSize {
		logger.Error(fmt.Errorf("file exceeds maximum upload size")).WithString("step", "validation").Log()
		return server.UploadRvtoolsToAssessment400JSONResponse{Message: fmt.Sprintf("file exceeds maximum upload size of %d MiB", maxUploadSize>>20)}, nil
	}

	if err := validateXLSXFile(tmpFile.Name()); err != nil {
		logger.Error(err).WithString("step", "validation").Log()
		var invalid *validator.ErrInvalidFile
		if errors.As(err, &invalid) {
			return server.UploadRvtoolsToAssessment400JSONResponse{Message: invalid.Error()}, nil
		}
		return server.UploadRvtoolsToAssessment500JSONResponse{Message: "failed to read file"}, nil
	}

	logger.Step("file_received").WithInt("file_size", int(n)).Log()

	jobArgs := model.RVToolsJobArgs{
		AssessmentID:  request.Id,
		OperationType: model.UpdateOperationType,
		File:          tmpFile.Name(),
		OrgID:         user.Organization,
		Username:      user.Username,
		FirstName:     user.FirstName,
		LastName:      user.LastName,
	}

	job, err := h.jobSrv.CreateRVToolsJob(ctx, jobArgs)
	if err != nil {
		logger.Error(err).Log()
		return server.UploadRvtoolsToAssessment500JSONResponse{Message: "failed to create job"}, nil
	}
	cleanup = false

	logger.Success().WithParam("job_id", job.Id).Log()

	return server.UploadRvtoolsToAssessment202JSONResponse(*job), nil
}

// ListAssessmentsV2 (GET /api/v2/assessments)
func (h *ServiceHandler) ListAssessmentsV2(ctx context.Context, request server.ListAssessmentsV2RequestObject) (server.ListAssessmentsV2ResponseObject, error) {
	logger := log.NewDebugLogger("assessment_v2_handler").
		WithContext(ctx).
		Operation("list_assessments").
		Build()

	filter := service.NewAssessmentFilter()
	if request.Params.SourceType != nil {
		filter.WithSource(mappers.SourceTypeFromApi(*request.Params.SourceType))
	}

	assessments, err := h.assessmentSrv.ListAssessments(ctx, filter)
	if err != nil {
		logger.Error(err).Log()
		return server.ListAssessmentsV2500JSONResponse{Message: fmt.Sprintf("failed to list assessments: %v", err)}, nil
	}

	response := make([]api.AssessmentV2Summary, len(assessments))
	for i, assessment := range assessments {
		response[i] = mappers.AssessmentSummaryToApi(assessment)
	}
	return server.ListAssessmentsV2200JSONResponse(response), nil
}

// UpdateAssessmentV2 (PATCH /api/v2/assessments/{id})
func (h *ServiceHandler) UpdateAssessmentV2(ctx context.Context, request server.UpdateAssessmentV2RequestObject) (server.UpdateAssessmentV2ResponseObject, error) {
	logger := log.NewDebugLogger("assessment_v2_handler").
		WithContext(ctx).
		Operation("update_assessment").
		WithUUID("assessment_id", request.Id).
		WithRequestBody("request_body", request.Body).
		Build()

	if request.Body == nil || request.Body.Name == nil {
		logger.Error(fmt.Errorf("empty request body")).Log()
		return server.UpdateAssessmentV2400JSONResponse{Message: "empty body"}, nil
	}

	if err := validator.ValidateName(*request.Body.Name); err != nil {
		logger.Error(err).Log()
		return server.UpdateAssessmentV2400JSONResponse{Message: err.Error()}, nil
	}

	updatedAssessment, err := h.assessmentSrv.UpdateAssessment(ctx, request.Id, request.Body.Name)
	if err != nil {
		logger.Error(err).Log()
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.UpdateAssessmentV2404JSONResponse{Message: err.Error()}, nil
		case *service.ErrForbidden:
			return server.UpdateAssessmentV2403JSONResponse{Message: err.Error()}, nil
		case *service.ErrAgentUpdateForbidden, *service.ErrDuplicateKey:
			return server.UpdateAssessmentV2400JSONResponse{Message: err.Error()}, nil
		default:
			return server.UpdateAssessmentV2500JSONResponse{Message: fmt.Sprintf("failed to update assessment: %v", err)}, nil
		}
	}

	logger.Success().
		WithString("updated_name", updatedAssessment.Name).
		WithString("source_type", updatedAssessment.SourceType).
		Log()

	return server.UpdateAssessmentV2200JSONResponse(mappers.AssessmentToV2API(updatedAssessment)), nil
}

// GetAssessmentV2 (GET /api/v2/assessments/{id})
func (h *ServiceHandler) GetAssessmentV2(ctx context.Context, request server.GetAssessmentV2RequestObject) (server.GetAssessmentV2ResponseObject, error) {
	logger := log.NewDebugLogger("assessment_v2_handler").
		WithContext(ctx).
		Operation("get_assessment").
		WithUUID("assessment_id", request.Id).
		Build()

	assessment, err := h.assessmentSrv.GetAssessment(ctx, request.Id)
	if err != nil {
		logger.Error(err).Log()
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.GetAssessmentV2404JSONResponse{Message: err.Error()}, nil
		case *service.ErrForbidden:
			return server.GetAssessmentV2403JSONResponse{Message: err.Error()}, nil
		default:
			return server.GetAssessmentV2500JSONResponse{Message: fmt.Sprintf("failed to get assessment: %v", err)}, nil
		}
	}

	return server.GetAssessmentV2200JSONResponse(mappers.AssessmentToV2API(assessment)), nil
}

// GetVcenter (GET /api/v2/assessments/{id}/vcenters/{vid})
func (h *ServiceHandler) GetVcenter(ctx context.Context, request server.GetVcenterRequestObject) (server.GetVcenterResponseObject, error) {
	logger := log.NewDebugLogger("assessment_v2_handler").
		WithContext(ctx).
		Operation("get_vcenter").
		WithUUID("assessment_id", request.Id).
		WithUUID("vcenter_id", request.Vid).
		Build()

	inv, err := h.assessmentSrv.GetVcenterInventory(ctx, request.Id, request.Vid)
	if err != nil {
		logger.Error(err).Log()
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.GetVcenter404JSONResponse{Message: err.Error()}, nil
		case *service.ErrForbidden:
			return server.GetVcenter403JSONResponse{Message: err.Error()}, nil
		default:
			return server.GetVcenter500JSONResponse{Message: fmt.Sprintf("failed to get assessment: %v", err)}, nil
		}
	}

	return server.GetVcenter200JSONResponse{
		Data:     mappers.InventoryToV2API(*inv),
		Clusters: mappers.AssessmentClustersToAPI(inv.Inventory.Clusters),
	}, nil
}

// GetVcenterCluster (GET /api/v2/assessments/{id}/vcenters/{vid}/clusters/{cid})
func (h *ServiceHandler) GetVcenterCluster(ctx context.Context, request server.GetVcenterClusterRequestObject) (server.GetVcenterClusterResponseObject, error) {
	logger := log.NewDebugLogger("assessment_v2_handler").
		WithContext(ctx).
		Operation("get_vcenter_cluster").
		WithUUID("assessment_id", request.Id).
		WithUUID("vcenter_id", request.Vid).
		WithString("cluster_id", request.Cid).
		Build()

	clusterInv, err := h.assessmentSrv.GetClusterInventory(ctx, request.Id, request.Vid, request.Cid)
	if err != nil {
		logger.Error(err).Log()
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.GetVcenterCluster404JSONResponse{Message: err.Error()}, nil
		case *service.ErrForbidden:
			return server.GetVcenterCluster403JSONResponse{Message: err.Error()}, nil
		default:
			return server.GetVcenterCluster500JSONResponse{Message: fmt.Sprintf("failed to get assessment: %v", err)}, nil
		}
	}

	return server.GetVcenterCluster200JSONResponse(mappers.ClusterInventoryToAPI(*clusterInv)), nil
}

// DeleteVCenter (DELETE /api/v2/assessments/{id}/vcenters/{vid}
func (h *ServiceHandler) DeleteVCenter(ctx context.Context, request server.DeleteVCenterRequestObject) (server.DeleteVCenterResponseObject, error) {
	logger := log.NewDebugLogger("assessment_v2_handler").
		WithContext(ctx).
		Operation("delete_vcenter_inventory").
		WithUUID("assessment_id", request.Id).
		WithUUID("vcenter_id", request.Vid).
		Build()

	if err := h.assessmentSrv.DeleteVCenterInventory(ctx, request.Id, request.Vid); err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.DeleteVCenter404JSONResponse{Message: err.Error()}, nil
		case *service.ErrForbidden:
			return server.DeleteVCenter403JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).Log()
			return server.DeleteVCenter500JSONResponse{Message: "failed to delete vcenter inventory"}, nil
		}
	}

	logger.Success().Log()
	return server.DeleteVCenter204Response{}, nil
}

// (DELETE /api/v2/assessments/{id})
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

// (POST /api/v2/assessments/{id}/share)
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

// (DELETE /api/v2/assessments/{id}/share)
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
	v.RegisterStructValidation(validator.AssessmentFormValidator(), api.AssessmentForm{})
	return v.Struct(data)
}
