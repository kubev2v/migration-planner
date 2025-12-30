package v1alpha1

import (
	"context"
	"fmt"

	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/pkg/log"
	"github.com/kubev2v/migration-planner/pkg/requestid"
)

// (POST /api/v1/sizing)
func (h *ServiceHandler) CalculateSizing(ctx context.Context, request server.CalculateSizingRequestObject) (server.CalculateSizingResponseObject, error) {
	logger := log.NewDebugLogger("sizer_handler").
		WithContext(ctx).
		Operation("calculate_sizing").
		Build()

	user := auth.MustHaveUser(ctx)
	logger.Step("extract_user").WithString("org_id", user.Organization).WithString("username", user.Username).Log()

	if request.Body == nil {
		logger.Error(fmt.Errorf("empty request body")).Log()
		return server.CalculateSizing400JSONResponse{Message: "empty body", RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	if err := h.sizerSrv.Health(); err != nil {
		return server.CalculateSizing503JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	logger.Step("sizing_calculation").
		WithString("org_id", user.Organization).
		WithString("username", user.Username).
		Log()

	res, err := h.sizerSrv.CalculateSizing(ctx, request.Body) // Todo: Consider adding validation and mapping to the request body
	if err != nil {
		logger.Error(err).Log()
		return server.CalculateSizing500JSONResponse{Message: fmt.Sprintf("failed to calculate sizing: %v", err), RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	logger.Success().
		WithString("org_id", user.Organization).
		WithString("username", user.Username).
		Log()

	return server.CalculateSizing200JSONResponse(*res), nil
}
