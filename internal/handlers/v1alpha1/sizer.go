package v1alpha1

import (
	"context"
	"fmt"

	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/handlers/v1alpha1/mappers"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/pkg/log"
	"github.com/kubev2v/migration-planner/pkg/requestid"
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
		return server.CalculateAssessmentClusterRequirements400JSONResponse{Message: "empty body", RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	assessmentID := request.Id
	clusterID := request.Body.ClusterId

	if clusterID == "" {
		logger.Error(fmt.Errorf("clusterId is required")).Log()
		return server.CalculateAssessmentClusterRequirements400JSONResponse{Message: "clusterId is required", RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	// Validate worker node sizes
	if request.Body.WorkerNodeCPU <= 0 || request.Body.WorkerNodeMemory <= 0 {
		logger.Error(fmt.Errorf("worker node size must be greater than zero: CPU=%d, Memory=%d", request.Body.WorkerNodeCPU, request.Body.WorkerNodeMemory)).Log()
		return server.CalculateAssessmentClusterRequirements400JSONResponse{Message: fmt.Sprintf("worker node size must be greater than zero: CPU=%d, Memory=%d", request.Body.WorkerNodeCPU, request.Body.WorkerNodeMemory), RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	// Validate CPU overcommit ratio
	validCpuRatios := map[string]bool{"1:1": true, "1:2": true, "1:4": true, "1:6": true}
	if !validCpuRatios[string(request.Body.CpuOverCommitRatio)] {
		logger.Error(fmt.Errorf("invalid CPU over-commit ratio: %s", request.Body.CpuOverCommitRatio)).Log()
		return server.CalculateAssessmentClusterRequirements400JSONResponse{Message: fmt.Sprintf("invalid CPU over-commit ratio: %s. Valid values are: 1:1, 1:2, 1:4, 1:6", request.Body.CpuOverCommitRatio), RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	// Validate memory overcommit ratio
	validMemoryRatios := map[string]bool{"1:1": true, "1:2": true, "1:4": true}
	if !validMemoryRatios[string(request.Body.MemoryOverCommitRatio)] {
		logger.Error(fmt.Errorf("invalid memory over-commit ratio: %s", request.Body.MemoryOverCommitRatio)).Log()
		return server.CalculateAssessmentClusterRequirements400JSONResponse{Message: fmt.Sprintf("invalid memory over-commit ratio: %s. Valid values are: 1:1, 1:2, 1:4", request.Body.MemoryOverCommitRatio), RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	// Validate SMT configuration if threads provided
	if request.Body.WorkerNodeThreads != nil {
		threads := *request.Body.WorkerNodeThreads
		if threads < request.Body.WorkerNodeCPU {
			logger.Error(fmt.Errorf("workerNodeThreads (%d) must be >= workerNodeCPU (%d)", threads, request.Body.WorkerNodeCPU)).Log()
			return server.CalculateAssessmentClusterRequirements400JSONResponse{
				Message:   fmt.Sprintf("workerNodeThreads (%d) must be >= workerNodeCPU (%d)", threads, request.Body.WorkerNodeCPU),
				RequestId: requestid.FromContextPtr(ctx),
			}, nil
		}
	}

	assessment, err := h.assessmentSrv.GetAssessment(ctx, assessmentID)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			logger.Error(err).WithUUID("assessment_id", assessmentID).Log()
			return server.CalculateAssessmentClusterRequirements404JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
		default:
			logger.Error(err).WithUUID("assessment_id", assessmentID).Log()
			return server.CalculateAssessmentClusterRequirements500JSONResponse{Message: fmt.Sprintf("failed to get assessment: %v", err), RequestId: requestid.FromContextPtr(ctx)}, nil
		}
	}

	if user.Username != assessment.Username || user.Organization != assessment.OrgID {
		message := fmt.Sprintf("forbidden to access assessment %s by user %s", assessmentID, user.Username)
		logger.Error(fmt.Errorf("authorization failed: %s", message)).Log()
		return server.CalculateAssessmentClusterRequirements403JSONResponse{Message: message, RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	if err := h.sizerSrv.Health(ctx); err != nil {
		logger.Error(err).Log()
		return server.CalculateAssessmentClusterRequirements503JSONResponse{Message: fmt.Sprintf("sizer service unavailable: %v", err), RequestId: requestid.FromContextPtr(ctx)}, nil
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
			return server.CalculateAssessmentClusterRequirements404JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
		case *service.ErrInvalidClusterInventory, *service.ErrInvalidRequest:
			logger.Error(err).WithUUID("assessment_id", assessmentID).WithString("cluster_id", clusterID).Log()
			return server.CalculateAssessmentClusterRequirements400JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
		default:
			logger.Error(err).Log()
			return server.CalculateAssessmentClusterRequirements500JSONResponse{Message: fmt.Sprintf("failed to calculate cluster requirements: %v", err), RequestId: requestid.FromContextPtr(ctx)}, nil
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
