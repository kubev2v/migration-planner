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
	"github.com/kubev2v/migration-planner/internal/store/model"
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

	var mainInventory model.AssessmentInventory
	vcenters := make([]api.AssessmentVcenter, 0, len(assessment.Inventories))

	for _, inventory := range assessment.Inventories {

		if !inventory.IsSubset {
			// Expect exactly one non subset inventory
			mainInventory = inventory
		}

		if inventory.VCenterID == "" {
			continue
		}

		if assessment.SourceType == string(api.AssessmentSourceTypeRvtools) {
			// skip aggregated inventory
			if !inventory.IsSubset {
				continue
			}
		} else {
			if inventory.IsSubset {
				continue
			}
		}

		_, clusters, err := mappers.AssessmentInventoryToV2(inventory)
		if err != nil {
			logger.Error(err).Log()
			return server.GetAssessmentV2500JSONResponse{
				Message: fmt.Sprintf("failed to read vcenter inventory: %v", err),
			}, nil
		}

		id, err := uuid.Parse(inventory.VCenterID)
		if err != nil {
			return server.GetAssessmentV2500JSONResponse{
				Message: fmt.Sprintf("failed to parse vcenter id: %v", err),
			}, nil
		}

		vcenter := api.AssessmentVcenter{
			Id:       id,
			Clusters: mappers.AssessmentClustersToAPI(clusters),
		}

		vcenters = append(vcenters, vcenter)
	}

	data, _, err := mappers.AssessmentInventoryToV2(mainInventory)
	if err != nil {
		logger.Error(err).Log()
		return server.GetAssessmentV2500JSONResponse{Message: err.Error()}, nil
	}

	return server.GetAssessmentV2200JSONResponse{
		Data:     data,
		Vcenters: &vcenters,
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

	row, ok := inventoryByVcenterID(assessment, request.Vid.String())
	if !ok {
		return server.GetVcenter404JSONResponse{Message: fmt.Sprintf("vcenter %s not found in assessment %s", request.Vid, request.Id)}, nil
	}
	vcenter, clusters, err := mappers.AssessmentInventoryToV2(row)
	if err != nil {
		logger.Error(err).Log()
		return server.GetVcenter500JSONResponse{Message: fmt.Sprintf("failed to read vcenter inventory: %v", err)}, nil
	}

	clusterList := mappers.AssessmentClustersToAPI(clusters)
	return server.GetVcenter200JSONResponse{Data: vcenter, Clusters: &clusterList}, nil
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

	row, ok := inventoryByVcenterID(assessment, request.Vid.String())
	if !ok {
		return server.GetVcenterCluster404JSONResponse{Message: fmt.Sprintf("vcenter %s not found in assessment %s", request.Vid, request.Id)}, nil
	}
	_, clusters, err := mappers.AssessmentInventoryToV2(row)
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

func inventoryByVcenterID(assessment *model.Assessment, id string) (model.AssessmentInventory, bool) {
	inventories := assessment.Inventories

	for _, inventory := range inventories {
		if inventory.VCenterID != id {
			continue
		}

		if assessment.SourceType != string(api.AssessmentSourceTypeRvtools) {
			if inventory.IsSubset {
				continue
			}
		}

		return inventory, true
	}

	return model.AssessmentInventory{}, false
}
