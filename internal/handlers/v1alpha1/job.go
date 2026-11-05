package v1alpha1

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/handlers/validator"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/pkg/log"
)

const maxUploadSize = 50 << 20 // 50 MiB

func validateXLSXFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer func() { _ = f.Close() }()

	header := make([]byte, 4)
	if _, err := io.ReadFull(f, header); err != nil {
		return validator.NewErrInvalidFile("file is not a valid Excel file")
	}
	return validator.ValidateXLSXMagicBytes(header)
}

// (GET /api/v1/assessments/jobs/{id})
func (h *ServiceHandler) GetJob(ctx context.Context, request server.GetJobRequestObject) (server.GetJobResponseObject, error) {
	logger := log.NewDebugLogger("job_handler").
		WithContext(ctx).
		Operation("get_job").
		WithParam("job_id", request.Id).
		Build()

	user := auth.MustHaveUser(ctx)
	logger.Step("extract_user").WithString("org_id", user.Organization).WithString("username", user.Username).Log()

	job, err := h.jobSrv.GetJob(ctx, request.Id, user.Organization, user.Username)
	if err != nil {
		switch err.(type) {
		case *service.ErrJobNotFound:
			logger.Error(err).Log()
			return server.GetJob404JSONResponse{Message: err.Error()}, nil
		case *service.ErrJobForbidden:
			logger.Error(err).Log()
			return server.GetJob403JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).Log()
			return server.GetJob500JSONResponse{Message: fmt.Sprintf("failed to get job: %v", err)}, nil
		}
	}

	logger.Success().WithString("status", string(job.Status)).Log()

	return server.GetJob200JSONResponse(*job), nil
}

// (DELETE /api/v1/assessments/jobs/{id})
func (h *ServiceHandler) CancelJob(ctx context.Context, request server.CancelJobRequestObject) (server.CancelJobResponseObject, error) {
	logger := log.NewDebugLogger("job_handler").
		WithContext(ctx).
		Operation("cancel_job").
		WithParam("job_id", request.Id).
		Build()

	user := auth.MustHaveUser(ctx)
	logger.Step("extract_user").WithString("org_id", user.Organization).WithString("username", user.Username).Log()

	job, err := h.jobSrv.CancelJob(ctx, request.Id, user.Organization, user.Username)
	if err != nil {
		switch err.(type) {
		case *service.ErrJobNotFound:
			logger.Error(err).Log()
			return server.CancelJob404JSONResponse{Message: err.Error()}, nil
		case *service.ErrJobForbidden:
			logger.Error(err).Log()
			return server.CancelJob403JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).Log()
			return server.CancelJob500JSONResponse{Message: fmt.Sprintf("failed to cancel job: %v", err)}, nil
		}
	}

	logger.Success().Log()

	return server.CancelJob200JSONResponse(*job), nil
}
