package v1alpha1

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/handlers/v1alpha1/mappers"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/pkg/log"
)

// CreateRVToolsAssessmentV2 (POST /api/v2/assessments/rvtools)
func (h *ServiceHandler) CreateRVToolsAssessmentV2(ctx context.Context, request server.CreateRVToolsAssessmentV2RequestObject) (server.CreateRVToolsAssessmentV2ResponseObject, error) {
	return server.CreateRVToolsAssessmentV2500JSONResponse{}, errors.New("not yet implemented")
}

// UploadRvtoolsToAssessment (PUT /api/v2/assessments/{id}/rvtools)
func (h *ServiceHandler) UploadRvtoolsToAssessment(ctx context.Context, request server.UploadRvtoolsToAssessmentRequestObject) (server.UploadRvtoolsToAssessmentResponseObject, error) {
	return server.UploadRvtoolsToAssessment500JSONResponse{}, errors.New("not yet implemented")
}

// ListAssessmentsV2 (GET /api/v2/assessments)
func (h *ServiceHandler) ListAssessmentsV2(ctx context.Context, _ server.ListAssessmentsV2RequestObject) (server.ListAssessmentsV2ResponseObject, error) {
	logger := log.NewDebugLogger("assessment_v2_handler").
		WithContext(ctx).
		Operation("list_assessments").
		Build()

	assessments, err := h.assessmentSrv.ListAssessments(ctx, service.NewAssessmentFilter())
	if err != nil {
		logger.Error(err).Log()
		return server.ListAssessmentsV2500JSONResponse{Message: fmt.Sprintf("failed to list assessments: %v", err)}, nil
	}

	response := make([]api.AssessmentV2Summary, len(assessments))
	for i, assessment := range assessments {
		response[i] = mappers.AssessmentV2ToApi(assessment)
	}
	return server.ListAssessmentsV2200JSONResponse(response), nil
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
		var errResourceNotFound *service.ErrResourceNotFound
		var errForbidden *service.ErrForbidden
		switch {
		case errors.As(err, &errResourceNotFound):
			return server.GetAssessmentV2404JSONResponse{Message: err.Error()}, nil
		case errors.As(err, &errForbidden):
			return server.GetAssessmentV2403JSONResponse{Message: err.Error()}, nil
		default:
			return server.GetAssessmentV2500JSONResponse{Message: fmt.Sprintf("failed to get assessment: %v", err)}, nil
		}
	}

	vcenterIDs := make([]string, 0, len(assessment.SubsetInventories)+1)
	vcenterIDs = append(vcenterIDs, assessment.MainInventory.VCenterID)
	for _, inventory := range assessment.SubsetInventories {
		vcenterIDs = append(vcenterIDs, inventory.VCenterID)
	}

	vcenters := make([]api.SubResource, 0, len(vcenterIDs))
	seen := make(map[string]struct{})

	for _, vcenterID := range vcenterIDs {
		if vcenterID == "" {
			continue
		}

		if _, ok := seen[vcenterID]; ok {
			continue
		}

		seen[vcenterID] = struct{}{}

		id, err := uuid.Parse(vcenterID)
		if err != nil {
			return server.GetAssessmentV2500JSONResponse{
				Message: fmt.Sprintf("failed to parse vcenter id: %v", err),
			}, nil
		}

		vcenter := api.SubResource{
			Id: id.String(),
		}

		vcenters = append(vcenters, vcenter)
	}

	data, _, err := mappers.AssessmentInventoryToAPI(assessment.MainInventory)
	if err != nil {
		logger.Error(err).Log()
		return server.GetAssessmentV2500JSONResponse{Message: err.Error()}, nil
	}

	return server.GetAssessmentV2200JSONResponse{
		Data:     data,
		Vcetners: vcenters,
	}, nil
}

// GetVcenter (GET /api/v2/assessments/{id}/vcenters/{vid})
func (h *ServiceHandler) GetVcenter(ctx context.Context, request server.GetVcenterRequestObject) (server.GetVcenterResponseObject, error) {
	logger := log.NewDebugLogger("assessment_v2_handler").
		WithContext(ctx).
		Operation("get_vcenter").
		WithUUID("assessment_id", request.Id).
		WithUUID("vcenter_id", request.Vid).
		Build()

	assessment, err := h.assessmentSrv.GetAssessment(ctx, request.Id)
	if err != nil {
		logger.Error(err).Log()
		var errResourceNotFound *service.ErrResourceNotFound
		var errForbidden *service.ErrForbidden
		switch {
		case errors.As(err, &errResourceNotFound):
			return server.GetVcenter404JSONResponse{Message: err.Error()}, nil
		case errors.As(err, &errForbidden):
			return server.GetVcenter403JSONResponse{Message: err.Error()}, nil
		default:
			return server.GetVcenter500JSONResponse{Message: fmt.Sprintf("failed to get assessment: %v", err)}, nil
		}
	}

	vcenter, clusters, err := mappers.AssessmentInventoryToAPI(assessment.MainInventory)
	if err != nil {
		logger.Error(err).Log()
		return server.GetVcenter500JSONResponse{Message: fmt.Sprintf("failed to read vcenter inventory: %v", err)}, nil
	}

	clusterList := mappers.AssessmentClustersToAPI(clusters)
	return server.GetVcenter200JSONResponse{Data: vcenter, Clusters: clusterList}, nil
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

	assessment, err := h.assessmentSrv.GetAssessment(ctx, request.Id)
	if err != nil {
		logger.Error(err).Log()
		var errResourceNotFound *service.ErrResourceNotFound
		var errForbidden *service.ErrForbidden
		switch {
		case errors.As(err, &errResourceNotFound):
			return server.GetVcenterCluster404JSONResponse{Message: err.Error()}, nil
		case errors.As(err, &errForbidden):
			return server.GetVcenterCluster403JSONResponse{Message: err.Error()}, nil
		default:
			return server.GetVcenterCluster500JSONResponse{Message: fmt.Sprintf("failed to get assessment: %v", err)}, nil
		}
	}

	_, clusters, err := mappers.AssessmentInventoryToAPI(assessment.MainInventory)
	if err != nil {
		logger.Error(err).Log()
		return server.GetVcenterCluster500JSONResponse{Message: fmt.Sprintf("failed to read vcenter inventory: %v", err)}, nil
	}
	cluster, ok := clusters[request.Cid]
	if !ok {
		return server.GetVcenterCluster404JSONResponse{Message: fmt.Sprintf("cluster %s not found in vcenter %s", request.Cid, request.Vid)}, nil
	}
	return server.GetVcenterCluster200JSONResponse(cluster), nil
}
