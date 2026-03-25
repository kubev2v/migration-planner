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

	// Validate worker node sizes
	if request.Body.WorkerNodeCPU <= 0 || request.Body.WorkerNodeMemory <= 0 {
		logger.Error(fmt.Errorf("worker node size must be greater than zero: CPU=%d, Memory=%d", request.Body.WorkerNodeCPU, request.Body.WorkerNodeMemory)).Log()
		return server.CalculateAssessmentClusterRequirements400JSONResponse{Message: fmt.Sprintf("worker node size must be greater than zero: CPU=%d, Memory=%d", request.Body.WorkerNodeCPU, request.Body.WorkerNodeMemory)}, nil
	}

	// Validate CPU overcommit ratio
	validCpuRatios := map[string]bool{"1:1": true, "1:2": true, "1:4": true, "1:6": true}
	if !validCpuRatios[string(request.Body.CpuOverCommitRatio)] {
		logger.Error(fmt.Errorf("invalid CPU over-commit ratio: %s", request.Body.CpuOverCommitRatio)).Log()
		return server.CalculateAssessmentClusterRequirements400JSONResponse{Message: fmt.Sprintf("invalid CPU over-commit ratio: %s. Valid values are: 1:1, 1:2, 1:4, 1:6", request.Body.CpuOverCommitRatio)}, nil
	}

	// Validate memory overcommit ratio
	validMemoryRatios := map[string]bool{"1:1": true, "1:2": true, "1:4": true}
	if !validMemoryRatios[string(request.Body.MemoryOverCommitRatio)] {
		logger.Error(fmt.Errorf("invalid memory over-commit ratio: %s", request.Body.MemoryOverCommitRatio)).Log()
		return server.CalculateAssessmentClusterRequirements400JSONResponse{Message: fmt.Sprintf("invalid memory over-commit ratio: %s. Valid values are: 1:1, 1:2, 1:4", request.Body.MemoryOverCommitRatio)}, nil
	}

	// Validate that control plane fields are not provided when hosted control plane is enabled
	if err := validateNoControlPlaneFieldsWhenHosted(request.Body); err != nil {
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

	// Validate SMT configuration if threads provided
	if request.Body.WorkerNodeThreads != nil {
		threads := *request.Body.WorkerNodeThreads
		if threads < request.Body.WorkerNodeCPU {
			logger.Error(fmt.Errorf("workerNodeThreads (%d) must be >= workerNodeCPU (%d)", threads, request.Body.WorkerNodeCPU)).Log()
			return server.CalculateAssessmentClusterRequirements400JSONResponse{
				Message: fmt.Sprintf("workerNodeThreads (%d) must be >= workerNodeCPU (%d)", threads, request.Body.WorkerNodeCPU),
			}, nil
		}
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

// validateNoControlPlaneFieldsWhenHosted checks that control plane fields are not provided
// when hosted control plane mode is enabled. Returns an error if any control plane field is present.
func validateNoControlPlaneFieldsWhenHosted(req *api.ClusterRequirementsRequest) error {
	if req.HostedControlPlane == nil || !*req.HostedControlPlane {
		return nil
	}

	// Check each control plane field directly (not through interface{} to avoid nil pointer issues)
	if req.ControlPlaneNodeCount != nil {
		return fmt.Errorf("controlPlaneNodeCount cannot be specified when hostedControlPlane is true")
	}
	if req.ControlPlaneCPU != nil {
		return fmt.Errorf("controlPlaneCPU cannot be specified when hostedControlPlane is true")
	}
	if req.ControlPlaneMemory != nil {
		return fmt.Errorf("controlPlaneMemory cannot be specified when hostedControlPlane is true")
	}
	if req.ControlPlaneSchedulable != nil {
		return fmt.Errorf("controlPlaneSchedulable cannot be specified when hostedControlPlane is true")
	}

	return nil
}
