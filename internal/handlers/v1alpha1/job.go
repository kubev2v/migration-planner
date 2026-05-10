package v1alpha1

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"

	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/handlers/validator"
	"github.com/kubev2v/migration-planner/internal/rvtools/jobs"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/pkg/log"
)

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

	// Parse multipart form data
	var name string
	var fileContent []byte

	// Helper to process a single part with deferred cleanup
	processPart := func(part *multipart.Part) error {
		defer func() { _ = part.Close() }()

		switch part.FormName() {
		case "name":
			nameBytes, err := io.ReadAll(part)
			if err != nil {
				return fmt.Errorf("failed to read name: %w", err)
			}
			name = string(nameBytes)
		case "file":
			buff := bytes.NewBuffer([]byte{})
			n, err := io.Copy(buff, part)
			if err != nil {
				return fmt.Errorf("failed to read file: %w", err)
			}
			if n == 0 {
				return fmt.Errorf("rvtools file is empty")
			}
			fileContent = buff.Bytes()
		}
		return nil
	}

	for {
		part, err := request.Body.NextPart()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			logger.Error(err).WithString("step", "parse_multipart").Log()
			return server.CreateRVToolsAssessment400JSONResponse{Message: fmt.Sprintf("failed to parse form: %v", err)}, nil
		}

		if err := processPart(part); err != nil {
			logger.Error(err).WithString("step", "process_part").Log()
			return server.CreateRVToolsAssessment400JSONResponse{Message: err.Error()}, nil
		}
	}

	if err := validator.ValidateName(name); err != nil {
		logger.Error(err).WithString("step", "validation").Log()
		return server.CreateRVToolsAssessment400JSONResponse{Message: err.Error()}, nil
	}
	if len(fileContent) == 0 {
		logger.Error(fmt.Errorf("file is required")).Log()
		return server.CreateRVToolsAssessment400JSONResponse{Message: "file is required"}, nil
	}
	if err := validator.ValidateXLSXMagicBytes(fileContent); err != nil {
		logger.Error(err).WithString("step", "validation").Log()
		return server.CreateRVToolsAssessment400JSONResponse{Message: err.Error()}, nil
	}

	logger.Step("file_read").WithInt("file_size", len(fileContent)).Log()

	// Create job args (file content is stored separately in rvtools_files table)
	jobArgs := jobs.RVToolsJobArgs{
		Name:      name,
		OrgID:     user.Organization,
		Username:  user.Username,
		FirstName: user.FirstName,
		LastName:  user.LastName,
	}

	// Create the job (service stores file and sets FileID)
	job, err := h.jobSrv.CreateRVToolsJob(ctx, jobArgs, fileContent)
	if err != nil {
		logger.Error(err).Log()
		return server.CreateRVToolsAssessment500JSONResponse{Message: fmt.Sprintf("failed to create job: %v", err)}, nil
	}

	logger.Success().WithParam("job_id", job.Id).Log()

	return server.CreateRVToolsAssessment202JSONResponse(*job), nil
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
