package v1alpha1

import (
	"context"
	"fmt"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/handlers/v1alpha1/mappers"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/pkg/log"
)

// Bounds must match api/v1alpha1/openapi.yaml (ClusterRequirementsRequest.workerNodeThreads).
const (
	workerNodeThreadsMin = 2
	workerNodeThreadsMax = 2000
)

var (
	validCPUOverCommitRatios = map[string]struct{}{
		"1:1": {}, "1:2": {}, "1:4": {}, "1:6": {}, "1:8": {},
	}
	validMemoryOverCommitRatios = map[string]struct{}{
		"1:1": {}, "1:2": {}, "1:4": {},
	}
)

func validateOverCommitRatios(cpu api.CpuOverCommitRatio, mem api.MemoryOverCommitRatio) error {
	if _, ok := validCPUOverCommitRatios[string(cpu)]; !ok {
		return fmt.Errorf(
			"invalid CPU over-commit ratio: %s. Valid values are: 1:1, 1:2, 1:4, 1:6, 1:8",
			cpu,
		)
	}
	if _, ok := validMemoryOverCommitRatios[string(mem)]; !ok {
		return fmt.Errorf(
			"invalid memory over-commit ratio: %s. Valid values are: 1:1, 1:2, 1:4",
			mem,
		)
	}
	return nil
}

func validateWorkerNodeThreads(threads *int, cpu int) error {
	if threads == nil {
		return nil
	}
	t := *threads
	switch {
	case t < workerNodeThreadsMin:
		return fmt.Errorf("workerNodeThreads must be at least %d, got: %d", workerNodeThreadsMin, t)
	case t > workerNodeThreadsMax:
		return fmt.Errorf("workerNodeThreads must be at most %d, got: %d", workerNodeThreadsMax, t)
	case t < cpu:
		return fmt.Errorf("workerNodeThreads (%d) must be >= workerNodeCPU (%d)", t, cpu)
	}
	return nil
}

// (GET /api/v1/assessments/{id}/cluster-requirements/stored-input)
func (h *ServiceHandler) GetAssessmentClusterRequirementsStoredInput(ctx context.Context, request server.GetAssessmentClusterRequirementsStoredInputRequestObject) (server.GetAssessmentClusterRequirementsStoredInputResponseObject, error) {
	logger := log.NewDebugLogger("sizer_handler").
		WithContext(ctx).
		Operation("get_stored_assessment_cluster_requirements").
		WithUUID("assessment_id", request.Id).
		Build()

	clusterID := request.Params.ClusterId

	if clusterID == "" {
		logger.Error(fmt.Errorf("clusterId is required")).Log()
		return server.GetAssessmentClusterRequirementsStoredInput400JSONResponse{Message: "clusterId is required"}, nil
	}

	user := auth.MustHaveUser(ctx)
	assessment, err := h.assessmentSrv.GetAssessment(ctx, request.Id)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			logger.Error(err).WithUUID("assessment_id", request.Id).Log()
			return server.GetAssessmentClusterRequirementsStoredInput404JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).WithUUID("assessment_id", request.Id).Log()
			return server.GetAssessmentClusterRequirementsStoredInput500JSONResponse{Message: fmt.Sprintf("failed to get assessment: %v", err)}, nil
		}
	}

	if user.Username != assessment.Username || user.Organization != assessment.OrgID {
		message := fmt.Sprintf("forbidden to access assessment %s by user %s", request.Id, user.Username)
		logger.Error(fmt.Errorf("authorization failed: %s", message)).Log()
		return server.GetAssessmentClusterRequirementsStoredInput403JSONResponse{Message: message}, nil
	}

	storedInput, err := h.sizerSrv.GetClusterRequirementsInput(ctx, request.Id, clusterID)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			logger.Error(err).WithUUID("assessment_id", request.Id).WithString("cluster_id", clusterID).Log()
			return server.GetAssessmentClusterRequirementsStoredInput404JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).WithUUID("assessment_id", request.Id).WithString("cluster_id", clusterID).Log()
			return server.GetAssessmentClusterRequirementsStoredInput500JSONResponse{Message: fmt.Sprintf("failed to get stored cluster requirements: %v", err)}, nil
		}
	}

	return server.GetAssessmentClusterRequirementsStoredInput200JSONResponse(mappers.ClusterRequirementsInputFormToAPI(*storedInput)), nil
}

// (POST /api/v1/assessments/{id}/cluster-requirements)
func (h *ServiceHandler) CalculateAssessmentClusterRequirements(ctx context.Context, request server.CalculateAssessmentClusterRequirementsRequestObject) (server.CalculateAssessmentClusterRequirementsResponseObject, error) {
	logger := log.NewDebugLogger("sizer_handler").
		WithContext(ctx).
		Operation("get_assessment_cluster_requirements").
		WithUUID("assessment_id", request.Id).
		Build()

	user := auth.MustHaveUser(ctx)
	logger.Step("extract_user").WithString("org_id", user.Organization).WithString("username", user.Username).Log()

	if request.Body == nil {
		logger.Error(fmt.Errorf("empty request body")).Log()
		return server.CalculateAssessmentClusterRequirements400JSONResponse{Message: "empty body"}, nil
	}

	assessmentID := request.Id
	clusterID := request.Body.ClusterId

	if clusterID == "" {
		logger.Error(fmt.Errorf("clusterId is required")).Log()
		return server.CalculateAssessmentClusterRequirements400JSONResponse{Message: "clusterId is required"}, nil
	}

	if request.Body.WorkerNodeCPU <= 0 || request.Body.WorkerNodeMemory <= 0 {
		logger.Error(fmt.Errorf("worker node size must be greater than zero: CPU=%d, Memory=%d", request.Body.WorkerNodeCPU, request.Body.WorkerNodeMemory)).Log()
		return server.CalculateAssessmentClusterRequirements400JSONResponse{Message: fmt.Sprintf("worker node size must be greater than zero: CPU=%d, Memory=%d", request.Body.WorkerNodeCPU, request.Body.WorkerNodeMemory)}, nil
	}

	if err := validateOverCommitRatios(request.Body.CpuOverCommitRatio, request.Body.MemoryOverCommitRatio); err != nil {
		logger.Error(err).Log()
		return server.CalculateAssessmentClusterRequirements400JSONResponse{Message: err.Error()}, nil
	}

	// Validate that control plane fields are not provided when hosted control plane is enabled
	if err := validateNoControlPlaneFieldsWhenHosted(
		request.Body.HostedControlPlane,
		request.Body.ControlPlaneNodeCount != nil,
		request.Body.ControlPlaneCPU != nil,
		request.Body.ControlPlaneMemory != nil,
		request.Body.ControlPlaneSchedulable != nil,
	); err != nil {
		logger.Error(err).Log()
		return server.CalculateAssessmentClusterRequirements400JSONResponse{Message: err.Error()}, nil
	}

	// Validate controlPlaneNodeCount (API enum handles this in production, but tests bypass middleware)
	if request.Body.HostedControlPlane == nil || !*request.Body.HostedControlPlane {
		if request.Body.ControlPlaneNodeCount != nil {
			count := int(*request.Body.ControlPlaneNodeCount)
			if count != 1 && count != 3 {
				logger.Error(fmt.Errorf("invalid controlPlaneNodeCount: %d", count)).Log()
				return server.CalculateAssessmentClusterRequirements400JSONResponse{
					Message: fmt.Sprintf("invalid controlPlaneNodeCount: %d", count),
				}, nil
			}
		}
	}

	if err := validateWorkerNodeThreads(request.Body.WorkerNodeThreads, request.Body.WorkerNodeCPU); err != nil {
		logger.Error(err).Log()
		return server.CalculateAssessmentClusterRequirements400JSONResponse{Message: err.Error()}, nil
	}

	_, err := h.assessmentSrv.GetAssessment(ctx, assessmentID)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			logger.Error(err).WithUUID("assessment_id", assessmentID).Log()
			return server.CalculateAssessmentClusterRequirements404JSONResponse{Message: err.Error()}, nil
		case *service.ErrForbidden:
			logger.Error(err).WithUUID("assessment_id", assessmentID).Log()
			return server.CalculateAssessmentClusterRequirements403JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).WithUUID("assessment_id", assessmentID).Log()
			return server.CalculateAssessmentClusterRequirements500JSONResponse{Message: fmt.Sprintf("failed to get assessment: %v", err)}, nil
		}
	}

	if err := h.sizerSrv.Health(ctx); err != nil {
		logger.Error(err).Log()
		return server.CalculateAssessmentClusterRequirements503JSONResponse{Message: fmt.Sprintf("sizer service unavailable: %v", err)}, nil
	}

	logger.Step("cluster_requirements_calculation").
		WithUUID("assessment_id", assessmentID).
		WithString("cluster_id", clusterID).
		WithString("org_id", user.Organization).
		WithString("username", user.Username).
		Log()

	// Convert API request to domain model
	domainRequest := mappers.ClusterRequirementsRequestToForm(*request.Body)

	res, err := h.sizerSrv.CalculateClusterRequirements(ctx, assessmentID, &domainRequest)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			logger.Error(err).WithUUID("assessment_id", assessmentID).Log()
			return server.CalculateAssessmentClusterRequirements404JSONResponse{Message: err.Error()}, nil
		case *service.ErrInvalidClusterInventory, *service.ErrInvalidRequest:
			logger.Error(err).WithUUID("assessment_id", assessmentID).WithString("cluster_id", clusterID).Log()
			return server.CalculateAssessmentClusterRequirements400JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).Log()
			return server.CalculateAssessmentClusterRequirements500JSONResponse{Message: fmt.Sprintf("failed to calculate cluster requirements: %v", err)}, nil
		}
	}

	logger.Success().
		WithString("org_id", user.Organization).
		WithString("username", user.Username).
		WithInt("total_nodes", res.ClusterSizing.TotalNodes).
		Log()

	// Convert domain model to API response
	apiResponse := mappers.ClusterRequirementsResponseFormToAPI(*res)
	return server.CalculateAssessmentClusterRequirements200JSONResponse(apiResponse), nil
}

func validateNoControlPlaneFieldsWhenHosted(
	hosted *bool,
	hasNodeCount, hasCPU, hasMemory, hasSchedulable bool,
) error {
	if hosted == nil || !*hosted {
		return nil
	}
	if hasNodeCount {
		return fmt.Errorf("controlPlaneNodeCount cannot be specified when hostedControlPlane is true")
	}
	if hasCPU {
		return fmt.Errorf("controlPlaneCPU cannot be specified when hostedControlPlane is true")
	}
	if hasMemory {
		return fmt.Errorf("controlPlaneMemory cannot be specified when hostedControlPlane is true")
	}
	if hasSchedulable {
		return fmt.Errorf("controlPlaneSchedulable cannot be specified when hostedControlPlane is true")
	}
	return nil
}

// (POST /api/v1/cluster-requirements)
func (h *ServiceHandler) CalculateClusterRequirements(
	ctx context.Context,
	request server.CalculateClusterRequirementsRequestObject,
) (server.CalculateClusterRequirementsResponseObject, error) {
	logger := log.NewDebugLogger("sizer_handler").
		WithContext(ctx).
		Operation("calculate_cluster_requirements").
		Build()

	user := auth.MustHaveUser(ctx)
	logger.Step("extract_user").
		WithString("org_id", user.Organization).
		WithString("username", user.Username).
		Log()

	if request.Body == nil {
		logger.Error(fmt.Errorf("empty request body")).Log()
		return server.CalculateClusterRequirements400JSONResponse{
			Message: "empty body",
		}, nil
	}

	if request.Body.TotalVMs <= 0 {
		logger.Error(fmt.Errorf("totalVMs must be greater than zero")).Log()
		return server.CalculateClusterRequirements400JSONResponse{
			Message: "totalVMs must be greater than zero",
		}, nil
	}
	if request.Body.TotalCPU <= 0 {
		logger.Error(fmt.Errorf("totalCPU must be greater than zero")).Log()
		return server.CalculateClusterRequirements400JSONResponse{
			Message: "totalCPU must be greater than zero",
		}, nil
	}
	if request.Body.TotalMemory <= 0 {
		logger.Error(fmt.Errorf("totalMemory must be greater than zero")).Log()
		return server.CalculateClusterRequirements400JSONResponse{
			Message: "totalMemory must be greater than zero",
		}, nil
	}

	if request.Body.WorkerNodeCPU <= 0 || request.Body.WorkerNodeMemory <= 0 {
		logger.Error(fmt.Errorf("worker node size must be greater than zero")).Log()
		return server.CalculateClusterRequirements400JSONResponse{
			Message: fmt.Sprintf(
				"worker node size must be greater than zero: CPU=%d, Memory=%d",
				request.Body.WorkerNodeCPU, request.Body.WorkerNodeMemory,
			),
		}, nil
	}

	if err := validateOverCommitRatios(request.Body.CpuOverCommitRatio, request.Body.MemoryOverCommitRatio); err != nil {
		logger.Error(err).Log()
		return server.CalculateClusterRequirements400JSONResponse{Message: err.Error()}, nil
	}

	if err := validateNoControlPlaneFieldsWhenHosted(
		request.Body.HostedControlPlane,
		request.Body.ControlPlaneNodeCount != nil,
		request.Body.ControlPlaneCPU != nil,
		request.Body.ControlPlaneMemory != nil,
		request.Body.ControlPlaneSchedulable != nil,
	); err != nil {
		logger.Error(err).Log()
		return server.CalculateClusterRequirements400JSONResponse{
			Message: err.Error(),
		}, nil
	}

	if request.Body.HostedControlPlane == nil || !*request.Body.HostedControlPlane {
		if request.Body.ControlPlaneNodeCount != nil {
			count := int(*request.Body.ControlPlaneNodeCount)
			if count != 1 && count != 3 {
				logger.Error(fmt.Errorf("invalid controlPlaneNodeCount")).Log()
				return server.CalculateClusterRequirements400JSONResponse{
					Message: fmt.Sprintf("invalid controlPlaneNodeCount: %d", count),
				}, nil
			}
		}
	}

	if err := validateWorkerNodeThreads(request.Body.WorkerNodeThreads, request.Body.WorkerNodeCPU); err != nil {
		logger.Error(err).Log()
		return server.CalculateClusterRequirements400JSONResponse{Message: err.Error()}, nil
	}

	// Check sizer service health
	if err := h.sizerSrv.Health(ctx); err != nil {
		logger.Error(err).Log()
		return server.CalculateClusterRequirements503JSONResponse{
			Message: fmt.Sprintf("sizer service unavailable: %v", err),
		}, nil
	}

	logger.Step("cluster_requirements_calculation").
		WithInt("total_vms", request.Body.TotalVMs).
		WithInt("total_cpu", request.Body.TotalCPU).
		WithInt("total_memory", request.Body.TotalMemory).
		WithString("org_id", user.Organization).
		WithString("username", user.Username).
		Log()

	// Convert API request to domain model
	domainRequest := mappers.StandaloneClusterRequirementsRequestToForm(*request.Body)

	// Calculate cluster requirements (no assessment context)
	res, err := h.sizerSrv.CalculateStandaloneClusterRequirements(ctx, &domainRequest)
	if err != nil {
		switch err.(type) {
		case *service.ErrInvalidRequest:
			logger.Error(err).Log()
			return server.CalculateClusterRequirements400JSONResponse{
				Message: err.Error(),
			}, nil
		default:
			logger.Error(err).Log()
			return server.CalculateClusterRequirements500JSONResponse{
				Message: fmt.Sprintf("failed to calculate cluster requirements: %v", err),
			}, nil
		}
	}

	logger.Success().
		WithString("org_id", user.Organization).
		WithString("username", user.Username).
		WithInt("total_nodes", res.ClusterSizing.TotalNodes).
		Log()

	apiResponse := mappers.StandaloneClusterRequirementsResponseFormToAPI(*res)
	return server.CalculateClusterRequirements200JSONResponse(apiResponse), nil
}
