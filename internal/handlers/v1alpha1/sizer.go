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

	err := h.sizerSrv.CalculateSizing()
	if err != nil {
		logger.Error(err).Log()
		return server.CalculateSizing500JSONResponse{Message: fmt.Sprintf("failed to calculate sizing: %v", err), RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	return server.CalculateSizing200JSONResponse{}, nil
}
