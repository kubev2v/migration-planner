package v1alpha1

import (
	"context"
	"fmt"
	"strings"

	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/handlers/v1alpha1/mappers"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/pkg/log"
)

// (POST /api/v1/assessments/{id}/cost-estimation)
func (h *ServiceHandler) CalculateCostEstimation(ctx context.Context, request server.CalculateCostEstimationRequestObject) (server.CalculateCostEstimationResponseObject, error) {
	logger := log.NewDebugLogger("cost_estimation_handler").
		WithContext(ctx).
		Operation("calculate_cost_estimation").
		WithUUID("assessment_id", request.Id).
		Build()

	user := auth.MustHaveUser(ctx)
	logger.Step("extract_user").WithString("org_id", user.Organization).WithString("username", user.Username).Log()

	if request.Body == nil {
		logger.Error(fmt.Errorf("empty request body")).Log()
		return server.CalculateCostEstimation400JSONResponse{Message: "empty body"}, nil
	}

	clusterID := request.Body.ClusterId
	if clusterID == "" {
		logger.Error(fmt.Errorf("clusterId is required")).Log()
		return server.CalculateCostEstimation400JSONResponse{Message: "clusterId is required"}, nil
	}

	// Map request to service form
	reqForm := &service.CostEstimationRequestForm{
		ClusterID: clusterID,
		Discounts: service.CostEstimationDiscountsForm{},
	}

	// Map discounts if provided
	if request.Body.Discounts != nil {
		if request.Body.Discounts.VcfDiscountPct != nil {
			reqForm.Discounts.VcfDiscountPct = *request.Body.Discounts.VcfDiscountPct
		}
		if request.Body.Discounts.VvfDiscountPct != nil {
			reqForm.Discounts.VvfDiscountPct = *request.Body.Discounts.VvfDiscountPct
		}
		if request.Body.Discounts.RedhatDiscountPct != nil {
			reqForm.Discounts.RedhatDiscountPct = *request.Body.Discounts.RedhatDiscountPct
		}
		if request.Body.Discounts.AapDiscountPct != nil {
			reqForm.Discounts.AapDiscountPct = *request.Body.Discounts.AapDiscountPct
		}
	}

	result, err := h.costEstimationSrv.CalculateCostEstimation(ctx, request.Id, reqForm)
	if err != nil {
		switch e := err.(type) {
		case *service.ErrResourceNotFound:
			logger.Error(err).Log()
			return server.CalculateCostEstimation404JSONResponse{Message: err.Error()}, nil
		case *service.ErrForbidden:
			logger.Error(err).Log()
			return server.CalculateCostEstimation403JSONResponse{Message: err.Error()}, nil
		case *service.ErrInvalidRequest:
			logger.Error(err).Log()
			return server.CalculateCostEstimation400JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).Log()
			// Check if it's a connection error to the cost estimation service
			if strings.Contains(e.Error(), "failed to call cost estimation service") {
				return server.CalculateCostEstimation503JSONResponse{Message: fmt.Sprintf("cost estimation service unavailable: %v", err)}, nil
			}
			return server.CalculateCostEstimation500JSONResponse{Message: fmt.Sprintf("failed to calculate cost estimation: %v", err)}, nil
		}
	}

	logger.Success().Log()

	return server.CalculateCostEstimation200JSONResponse(mappers.CostEstimationResponseToAPI(result)), nil
}
