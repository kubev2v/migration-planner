package v1alpha1

import (
	"context"
	"fmt"
	"io"

	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/handlers/v1alpha1/mappers"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/pkg/log"
	"github.com/kubev2v/migration-planner/pkg/requestid"
)

func (h *ServiceHandler) CreateRVToolsJob(ctx context.Context, request server.CreateRVToolsJobRequestObject) (server.CreateRVToolsJobResponseObject, error) {
	logger := log.NewDebugLogger("job_handler").WithContext(ctx).Operation("create_rvtools_job").Build()

	user := auth.MustHaveUser(ctx)

	if request.Body == nil {
		return server.CreateRVToolsJob400JSONResponse{Message: "empty body", RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	var fileData []byte
	for {
		part, err := request.Body.NextPart()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			logger.Error(err).Log()
			return server.CreateRVToolsJob400JSONResponse{Message: fmt.Sprintf("failed to read multipart form: %v", err), RequestId: requestid.FromContextPtr(ctx)}, nil
		}

		if part.FormName() == "file" {
			fileData, err = io.ReadAll(part)
			if err != nil {
				logger.Error(err).Log()
				return server.CreateRVToolsJob400JSONResponse{Message: fmt.Sprintf("failed to read file: %v", err), RequestId: requestid.FromContextPtr(ctx)}, nil
			}
		}
	}

	if len(fileData) == 0 {
		return server.CreateRVToolsJob400JSONResponse{Message: "file is required", RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	jobInfo, err := h.jobSrv.CreateRVToolsJob(ctx, fileData, &user)
	if err != nil {
		logger.Error(err).Log()
		return server.CreateRVToolsJob500JSONResponse{Message: fmt.Sprintf("failed to create job: %v", err), RequestId: requestid.FromContextPtr(ctx)}, nil
	}

	logger.Success().WithParam("job_id", jobInfo.ID).Log()
	return server.CreateRVToolsJob201JSONResponse(mappers.JobInfoToApi(jobInfo.ID, jobInfo.Status, jobInfo.Error)), nil
}

func (h *ServiceHandler) GetJob(ctx context.Context, request server.GetJobRequestObject) (server.GetJobResponseObject, error) {
	logger := log.NewDebugLogger("job_handler").WithContext(ctx).Operation("get_job").WithParam("job_id", request.Id).Build()

	user := auth.MustHaveUser(ctx)

	jobInfo, err := h.jobSrv.GetJob(ctx, request.Id, &user)
	if err != nil {
		logger.Error(err).Log()
		switch err.(type) {
		case *service.ErrJobNotFound:
			return server.GetJob404JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
		case *service.ErrJobAccessForbidden:
			return server.GetJob403JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
		default:
			return server.GetJob500JSONResponse{Message: fmt.Sprintf("failed to get job: %v", err), RequestId: requestid.FromContextPtr(ctx)}, nil
		}
	}

	logger.Success().Log()
	return server.GetJob200JSONResponse(mappers.JobInfoToApi(jobInfo.ID, jobInfo.Status, jobInfo.Error)), nil
}

func (h *ServiceHandler) CancelJob(ctx context.Context, request server.CancelJobRequestObject) (server.CancelJobResponseObject, error) {
	logger := log.NewDebugLogger("job_handler").WithContext(ctx).Operation("cancel_job").WithParam("job_id", request.Id).Build()

	user := auth.MustHaveUser(ctx)

	jobInfo, err := h.jobSrv.CancelJob(ctx, request.Id, &user)
	if err != nil {
		logger.Error(err).Log()
		switch err.(type) {
		case *service.ErrJobNotFound:
			return server.CancelJob404JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
		case *service.ErrJobAccessForbidden:
			return server.CancelJob403JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
		case *service.ErrJobAlreadyCompleted:
			return server.CancelJob409JSONResponse{Message: err.Error(), RequestId: requestid.FromContextPtr(ctx)}, nil
		default:
			return server.CancelJob500JSONResponse{Message: fmt.Sprintf("failed to cancel job: %v", err), RequestId: requestid.FromContextPtr(ctx)}, nil
		}
	}

	logger.Success().Log()
	return server.CancelJob200JSONResponse(mappers.JobInfoToApi(jobInfo.ID, jobInfo.Status, jobInfo.Error)), nil
}
