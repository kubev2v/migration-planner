package v1alpha1

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/handlers/v1alpha1/mappers"
	"github.com/kubev2v/migration-planner/internal/handlers/validator"
	"github.com/kubev2v/migration-planner/internal/inventorybundle"
	"github.com/kubev2v/migration-planner/internal/service"
	srvMappers "github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/log"
)

// validateSourceData validates the source data using the source validation rules
func validateSourceData(data interface{}) error {
	v := validator.NewValidator()
	v.Register(validator.NewSourceValidationRules()...)
	return validator.TransformValidationError(v.Struct(data))
}

// (GET /api/v1/sources)
func (s *ServiceHandler) ListSources(ctx context.Context, request server.ListSourcesRequestObject) (server.ListSourcesResponseObject, error) {
	user := auth.MustHaveUser(ctx)

	filter := service.NewSourceFilter(service.WithUsername(user.Username), service.WithOrgID(user.Organization))

	sources, err := s.sourceSrv.ListSources(ctx, filter)
	if err != nil {
		return server.ListSources500JSONResponse{}, nil
	}

	return server.ListSources200JSONResponse(mappers.SourceListToApi(sources)), nil
}

// (POST /api/v1/sources)
func (s *ServiceHandler) CreateSource(ctx context.Context, request server.CreateSourceRequestObject) (server.CreateSourceResponseObject, error) {
	if request.Body == nil {
		return server.CreateSource400JSONResponse{Message: "empty body"}, nil
	}

	form := v1alpha1.SourceCreate(*request.Body)
	if err := validateSourceData(form); err != nil {
		return server.CreateSource400JSONResponse{Message: err.Error()}, nil
	}

	user := auth.MustHaveUser(ctx)
	sourceCreateForm := mappers.SourceFormApi(form)
	sourceCreateForm.Username = user.Username
	sourceCreateForm.OrgID = user.Organization
	sourceCreateForm.EmailDomain = user.EmailDomain

	source, err := s.sourceSrv.CreateSource(ctx, sourceCreateForm)
	if err != nil {
		var dupErr *service.ErrDuplicateKey
		if errors.As(err, &dupErr) {
			return server.CreateSource400JSONResponse{Message: fmt.Sprintf("failed to create source: %v", err)}, nil
		}
		return server.CreateSource500JSONResponse{Message: fmt.Sprintf("failed to create source: %v", err)}, nil
	}

	response, err := mappers.SourceToApi(source)
	if err != nil {
		return server.CreateSource500JSONResponse{Message: fmt.Sprintf("failed to map source to api: %v", err)}, nil
	}

	return server.CreateSource201JSONResponse(response), nil
}

// (DELETE /api/v1/sources/{id})
func (s *ServiceHandler) DeleteSource(ctx context.Context, request server.DeleteSourceRequestObject) (server.DeleteSourceResponseObject, error) {
	source, err := s.sourceSrv.GetSource(ctx, request.Id)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.DeleteSource404JSONResponse{Message: err.Error()}, nil
		default:
			return server.DeleteSource500JSONResponse{}, nil
		}
	}

	user := auth.MustHaveUser(ctx)
	if user.Username != source.Username || user.Organization != source.OrgID {
		message := fmt.Sprintf("forbidden to delete source %s by user with org_id %s", request.Id, user.Organization)
		return server.DeleteSource403JSONResponse{Message: message}, nil
	}

	if err := s.sourceSrv.DeleteSource(ctx, request.Id); err != nil {
		return server.DeleteSource500JSONResponse{Message: fmt.Sprintf("failed to delete source: %v", err)}, nil
	}

	return server.DeleteSource200JSONResponse{}, nil
}

// (GET /api/v1/sources/{id})
func (s *ServiceHandler) GetSource(ctx context.Context, request server.GetSourceRequestObject) (server.GetSourceResponseObject, error) {
	source, err := s.sourceSrv.GetSource(ctx, request.Id)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.GetSource404JSONResponse{Message: err.Error()}, nil
		default:
			return server.GetSource500JSONResponse{}, nil
		}
	}

	user := auth.MustHaveUser(ctx)
	if user.Username != source.Username || user.Organization != source.OrgID {
		message := fmt.Sprintf("forbidden to access source %s by user %s", request.Id, user.Username)
		return server.GetSource403JSONResponse{Message: message}, nil
	}

	response, err := mappers.SourceToApi(*source)
	if err != nil {
		return server.GetSource500JSONResponse{Message: fmt.Sprintf("failed to map source to api: %v", err)}, nil
	}

	return server.GetSource200JSONResponse(response), nil
}

// (PUT /api/v1/sources/{id})
func (s *ServiceHandler) UpdateSource(ctx context.Context, request server.UpdateSourceRequestObject) (server.UpdateSourceResponseObject, error) {
	if request.Body == nil {
		return server.UpdateSource400JSONResponse{Message: "There is nothing to update"}, nil
	}

	if err := validateSourceData(*request.Body); err != nil {
		return server.UpdateSource400JSONResponse{Message: err.Error()}, nil
	}

	source, err := s.sourceSrv.GetSource(ctx, request.Id)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.UpdateSource404JSONResponse{Message: err.Error()}, nil
		default:
			return server.UpdateSource500JSONResponse{}, nil
		}
	}

	user := auth.MustHaveUser(ctx)
	if user.Username != source.Username || user.Organization != source.OrgID {
		message := fmt.Sprintf("forbidden to update source %s by user %s", request.Id, user.Username)
		return server.UpdateSource403JSONResponse{Message: message}, nil
	}

	// Convert API request to service form using handler mapper
	form := mappers.SourceUpdateFormApi(*request.Body)

	updatedSource, err := s.sourceSrv.UpdateSource(ctx, request.Id, form)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.UpdateSource404JSONResponse{Message: err.Error()}, nil
		default:
			return server.UpdateSource500JSONResponse{Message: fmt.Sprintf("failed to update source %s: %v", request.Id, err)}, nil
		}
	}

	response, err := mappers.SourceToApi(*updatedSource)
	if err != nil {
		return server.UpdateSource500JSONResponse{Message: fmt.Sprintf("failed to map source to api: %v", err)}, nil
	}

	return server.UpdateSource200JSONResponse(response), nil
}

// (PUT /api/v1/sources/{id}/inventory)
func (s *ServiceHandler) UpdateInventory(ctx context.Context, request server.UpdateInventoryRequestObject) (server.UpdateInventoryResponseObject, error) {
	logger := log.NewDebugLogger("source_handler").
		WithContext(ctx).
		Operation("update_inventory").
		WithUUID("source_id", request.Id).
		Build()

	// Route based on content type
	if request.MultipartBody != nil {
		return s.updateInventoryMultipart(ctx, request.Id, request.MultipartBody, logger)
	}
	if request.JSONBody != nil {
		return s.updateInventoryJSON(ctx, request.Id, request.JSONBody, logger)
	}
	return server.UpdateInventory400JSONResponse{Message: "empty body"}, nil
}

func (s *ServiceHandler) authorizeSourceAccess(ctx context.Context, sourceID uuid.UUID, logger *log.OperationTracer) (*model.Source, server.UpdateInventoryResponseObject, error) {
	source, err := s.sourceSrv.GetSource(ctx, sourceID)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return nil, server.UpdateInventory404JSONResponse{Message: err.Error()}, nil
		default:
			logger.Error(err).WithString("step", "get_source").Log()
			return nil, server.UpdateInventory500JSONResponse{
				Message: fmt.Sprintf("failed to get source %s: %v", sourceID, err),
			}, nil
		}
	}

	user := auth.MustHaveUser(ctx)
	if user.Organization != source.OrgID || user.Username != source.Username {
		message := fmt.Sprintf("forbidden to update inventory for source %s by user %s with org_id %s", sourceID, user.Username, user.Organization)
		return nil, server.UpdateInventory403JSONResponse{Message: message}, nil
	}

	return source, nil, nil
}

// inventoryUpdateResponse handles the common logic for updating inventory and returning the response
func (s *ServiceHandler) inventoryUpdateResponse(ctx context.Context, sourceID uuid.UUID, form srvMappers.InventoryUpdateForm) (server.UpdateInventoryResponseObject, error) {
	updatedSource, err := s.sourceSrv.UpdateInventory(ctx, form)
	if err != nil {
		switch err.(type) {
		case *service.ErrInvalidVCenterID:
			return server.UpdateInventory400JSONResponse{Message: err.Error()}, nil
		default:
			return server.UpdateInventory500JSONResponse{Message: fmt.Sprintf("failed to update source inventory %s: %v", sourceID, err)}, nil
		}
	}

	response, err := mappers.SourceToApi(updatedSource)
	if err != nil {
		return server.UpdateInventory500JSONResponse{Message: fmt.Sprintf("failed to map source to api: %v", err)}, nil
	}

	return server.UpdateInventory200JSONResponse(response), nil
}

func (s *ServiceHandler) updateInventoryJSON(ctx context.Context, sourceID uuid.UUID, body *v1alpha1.UpdateInventoryJSONRequestBody, logger *log.OperationTracer) (server.UpdateInventoryResponseObject, error) {
	_, errResponse, err := s.authorizeSourceAccess(ctx, sourceID, logger)
	if err != nil || errResponse != nil {
		return errResponse, err
	}

	data, err := json.Marshal(body.Inventory)
	if err != nil {
		return server.UpdateInventory500JSONResponse{Message: fmt.Sprintf("failed to update source inventory %s: %v", sourceID, err)}, nil
	}

	form := srvMappers.InventoryUpdateForm{
		AgentID:   body.AgentId,
		SourceID:  sourceID,
		Inventory: data,
		VCenterID: body.Inventory.VcenterId,
		Subsets:   []srvMappers.SourceSubsetUpdateForm{}, // JSON uploads have no subsets
	}

	return s.inventoryUpdateResponse(ctx, sourceID, form)
}

func (s *ServiceHandler) updateInventoryMultipart(ctx context.Context, sourceID uuid.UUID, body *multipart.Reader, logger *log.OperationTracer) (server.UpdateInventoryResponseObject, error) {
	source, errResponse, err := s.authorizeSourceAccess(ctx, sourceID, logger)
	if err != nil || errResponse != nil {
		return errResponse, err
	}

	fileBytes, err := readUploadedInventoryFile(body)
	if err != nil {
		logger.Error(err).WithString("step", "parse_multipart").Log()
		return server.UpdateInventory400JSONResponse{Message: err.Error()}, nil
	}

	parsed, err := inventorybundle.Parse(fileBytes)
	if err != nil {
		logger.Error(err).WithString("step", "parse_inventory").Log()
		return server.UpdateInventory400JSONResponse{Message: err.Error()}, nil
	}

	agentID := uuid.New()
	if parsed.AgentID != nil {
		agentID = *parsed.AgentID
	} else if len(source.Agents) > 0 {
		agentID = source.Agents[0].ID
	}

	form := srvMappers.InventoryUpdateForm{
		AgentID:   agentID,
		SourceID:  sourceID,
		Inventory: parsed.MainInventory,
		VCenterID: parsed.VCenterID,
		Subsets:   make([]srvMappers.SourceSubsetUpdateForm, len(parsed.Subsets)),
	}
	for i, subset := range parsed.Subsets {
		form.Subsets[i] = srvMappers.SourceSubsetUpdateForm{
			ID:        subset.ID,
			Name:      subset.Name,
			SourceID:  sourceID,
			VCenterID: subset.VCenterID,
			VMsCount:  subset.VMsCount,
			Inventory: subset.Inventory,
		}
	}

	logger.Success().WithInt("subset_count", len(form.Subsets)).Log()
	return s.inventoryUpdateResponse(ctx, sourceID, form)
}

func readUploadedInventoryFile(body *multipart.Reader) ([]byte, error) {
	var fileBytes []byte
	for {
		part, err := body.NextPart()
		if err != nil {
			if err == io.EOF {
				break
			}
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				return nil, fmt.Errorf("file exceeds maximum upload size of %d MiB", inventorybundle.MaxFileSize>>20)
			}
			return nil, fmt.Errorf("failed to parse form: %w", err)
		}

		if part.FormName() != "file" {
			_, _ = io.Copy(io.Discard, io.LimitReader(part, 1<<20))
			_ = part.Close()
			continue
		}

		data, err := io.ReadAll(io.LimitReader(part, inventorybundle.MaxFileSize+1))
		_ = part.Close()
		if err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				return nil, fmt.Errorf("file exceeds maximum upload size of %d MiB", inventorybundle.MaxFileSize>>20)
			}
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
		if int64(len(data)) > inventorybundle.MaxFileSize {
			return nil, fmt.Errorf("file exceeds maximum upload size of %d MiB", inventorybundle.MaxFileSize>>20)
		}
		fileBytes = data
	}
	if len(fileBytes) == 0 {
		return nil, fmt.Errorf("file is required")
	}
	return fileBytes, nil
}

// (HEAD /api/v1/sources/{id}/image)
func (s *ServiceHandler) HeadImage(ctx context.Context, request server.HeadImageRequestObject) (server.HeadImageResponseObject, error) {
	return nil, nil
}

// (GET /api/v1/sources/{id}/image-url)
func (s *ServiceHandler) GetSourceDownloadURL(ctx context.Context, request server.GetSourceDownloadURLRequestObject) (server.GetSourceDownloadURLResponseObject, error) {
	source, err := s.sourceSrv.GetSource(ctx, request.Id)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.GetSourceDownloadURL404JSONResponse{Message: err.Error()}, nil
		default:
			return server.GetSourceDownloadURL500JSONResponse{Message: fmt.Sprintf("failed to load source %s: %v", request.Id, err)}, nil
		}
	}

	user := auth.MustHaveUser(ctx)
	if user.Username != source.Username || user.Organization != source.OrgID {
		message := fmt.Sprintf("forbidden to access source %s by user with org_id %s", request.Id, user.Organization)
		return server.GetSourceDownloadURL403JSONResponse{Message: message}, nil
	}

	url, expireAt, err := s.sourceSrv.GetSourceDownloadURL(ctx, request.Id)
	if err != nil {
		return server.GetSourceDownloadURL500JSONResponse{Message: fmt.Sprintf("failed to get download URL for source %s: %v", request.Id, err)}, nil
	}
	return server.GetSourceDownloadURL200JSONResponse{Url: url, ExpiresAt: &expireAt}, nil
}

// (GET /health)
func (s *ServiceHandler) Health(ctx context.Context, request server.HealthRequestObject) (server.HealthResponseObject, error) {
	return server.Health200Response{}, nil
}
