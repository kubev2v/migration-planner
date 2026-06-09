package v1alpha1

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/handlers/validator"
	"github.com/kubev2v/migration-planner/internal/rvtools/jobs"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/pkg/log"
)

const maxUploadSize = 50 << 20 // 50 MiB

// (POST /api/v1/assessments/rvtools)
func (h *ServiceHandler) CreateRVToolsAssessment(ctx context.Context, request server.CreateRVToolsAssessmentRequestObject) (server.CreateRVToolsAssessmentResponseObject, error) {
	logger := log.NewDebugLogger("job_handler").
		WithContext(ctx).
		Operation("create_rvtools_assessment").
		Build()

	user := auth.MustHaveUser(ctx)
	logger.Step("extract_user").WithString("org_id", user.Organization).WithString("username", user.Username).Log()

	if request.Body == nil {
		logger.Error(fmt.Errorf("empty request body")).Log()
		return server.CreateRVToolsAssessment400JSONResponse{Message: "empty body"}, nil
	}

	var name string
	var tempFilePath string
	var fileSize int64
	cleanup := true
	defer func() {
		if cleanup && tempFilePath != "" {
			_ = os.Remove(tempFilePath)
		}
	}()

partsLoop:
	for {
		part, err := request.Body.NextPart()
		if err != nil {
			if err == io.EOF {
				break
			}
			logger.Error(err).WithString("step", "parse_multipart").Log()
			return server.CreateRVToolsAssessment400JSONResponse{Message: fmt.Sprintf("failed to parse form: %v", err)}, nil
		}

		switch part.FormName() {
		case "name":
			nameBytes, err := io.ReadAll(io.LimitReader(part, 1024))
			_ = part.Close()
			if err != nil {
				logger.Error(err).WithString("step", "read_name").Log()
				return server.CreateRVToolsAssessment400JSONResponse{Message: fmt.Sprintf("failed to read name: %v", err)}, nil
			}
			name = string(nameBytes)
		case "file":
			tmpFile, err := os.CreateTemp("", "rvtools-upload-*.xlsx")
			if err != nil {
				_ = part.Close()
				logger.Error(err).WithString("step", "create_temp_file").Log()
				return server.CreateRVToolsAssessment500JSONResponse{Message: "failed to create temp file"}, nil
			}
			n, copyErr := io.Copy(tmpFile, io.LimitReader(part, maxUploadSize+1))
			closeErr := tmpFile.Close()
			_ = part.Close()
			if copyErr != nil {
				_ = os.Remove(tmpFile.Name())
				logger.Error(copyErr).WithString("step", "write_temp_file").Log()
				return server.CreateRVToolsAssessment400JSONResponse{Message: fmt.Sprintf("failed to read file: %v", copyErr)}, nil
			}
			if closeErr != nil {
				_ = os.Remove(tmpFile.Name())
				logger.Error(closeErr).WithString("step", "close_temp_file").Log()
				return server.CreateRVToolsAssessment500JSONResponse{Message: "failed to write temp file"}, nil
			}
			if n == 0 {
				_ = os.Remove(tmpFile.Name())
				logger.Error(fmt.Errorf("rvtools file is empty")).WithString("step", "validation").Log()
				return server.CreateRVToolsAssessment400JSONResponse{Message: "rvtools file is empty"}, nil
			}
			if n > maxUploadSize {
				_ = os.Remove(tmpFile.Name())
				logger.Error(fmt.Errorf("file exceeds maximum upload size")).WithString("step", "validation").Log()
				return server.CreateRVToolsAssessment400JSONResponse{Message: fmt.Sprintf("file exceeds maximum upload size of %d MiB", maxUploadSize>>20)}, nil
			}
			tempFilePath = tmpFile.Name()
			fileSize = n
			break partsLoop
		default:
			_ = part.Close()
		}
	}

	if err := validator.ValidateName(name); err != nil {
		logger.Error(err).WithString("step", "validation").Log()
		return server.CreateRVToolsAssessment400JSONResponse{Message: err.Error()}, nil
	}
	if tempFilePath == "" {
		logger.Error(fmt.Errorf("file is required")).Log()
		return server.CreateRVToolsAssessment400JSONResponse{Message: "file is required"}, nil
	}
	if err := validateXLSXFile(tempFilePath); err != nil {
		logger.Error(err).WithString("step", "validation").Log()
		return server.CreateRVToolsAssessment400JSONResponse{Message: err.Error()}, nil
	}

	logger.Step("file_received").WithInt("file_size", int(fileSize)).Log()

	jobArgs := jobs.RVToolsJobArgs{
		Name:      name,
		FilePath:  tempFilePath,
		OrgID:     user.Organization,
		Username:  user.Username,
		FirstName: user.FirstName,
		LastName:  user.LastName,
	}

	job, err := h.jobSrv.CreateRVToolsJob(ctx, jobArgs)
	if err != nil {
		logger.Error(err).Log()
		return server.CreateRVToolsAssessment500JSONResponse{Message: fmt.Sprintf("failed to create job: %v", err)}, nil
	}
	cleanup = false

	logger.Success().WithParam("job_id", job.Id).Log()

	return server.CreateRVToolsAssessment202JSONResponse(*job), nil
}

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
