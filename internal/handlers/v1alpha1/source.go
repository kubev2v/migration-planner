package v1alpha1

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/api/server"
	apiServer "github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/handlers/v1alpha1/mappers"
	"github.com/kubev2v/migration-planner/internal/handlers/validator"
	"github.com/kubev2v/migration-planner/internal/service"
	srvMappers "github.com/kubev2v/migration-planner/internal/service/mappers"
)

type ServiceHandler struct {
	sourceSrv     *service.SourceService
	assessmentSrv *service.AssessmentService
}

func NewServiceHandler(sourceService *service.SourceService, a *service.AssessmentService) *ServiceHandler {
	return &ServiceHandler{
		sourceSrv:     sourceService,
		assessmentSrv: a,
	}
}

// validateSourceData validates the source data using the source validation rules
func validateSourceData(data interface{}) error {
	v := validator.NewValidator()
	v.Register(validator.NewSourceValidationRules()...)
	return v.Struct(data)
}

// (GET /api/v1/sources)
func (s *ServiceHandler) ListSources(ctx context.Context, request apiServer.ListSourcesRequestObject) (apiServer.ListSourcesResponseObject, error) {
	user := auth.MustHaveUser(ctx)

	filter := service.NewSourceFilter(service.WithUsername(user.Username), service.WithOrgID(user.Organization))

	sources, err := s.sourceSrv.ListSources(ctx, filter)
	if err != nil {
		return server.ListSources500JSONResponse{}, nil
	}

	return server.ListSources200JSONResponse(mappers.SourceListToApi(sources)), nil
}

// (POST /api/v1/sources)
func (s *ServiceHandler) CreateSource(ctx context.Context, request apiServer.CreateSourceRequestObject) (apiServer.CreateSourceResponseObject, error) {
	if request.Body == nil {
		return server.CreateSource400JSONResponse{Message: "empty body"}, nil
	}

	form := v1alpha1.SourceCreate(*request.Body)
	if err := validateSourceData(form); err != nil {
		return apiServer.CreateSource400JSONResponse{Message: err.Error()}, nil
	}

	user := auth.MustHaveUser(ctx)
	sourceCreateForm := mappers.SourceFormApi(form)
	sourceCreateForm.Username = user.Username
	sourceCreateForm.OrgID = user.Organization
	sourceCreateForm.EmailDomain = user.EmailDomain

	source, err := s.sourceSrv.CreateSource(ctx, sourceCreateForm)
	if err != nil {
		return apiServer.CreateSource500JSONResponse{Message: fmt.Sprintf("failed to create source: %v", err)}, nil
	}

	return apiServer.CreateSource201JSONResponse(mappers.SourceToApi(source)), nil
}

// (DELETE /api/v1/sources)
func (s *ServiceHandler) DeleteSources(ctx context.Context, request apiServer.DeleteSourcesRequestObject) (apiServer.DeleteSourcesResponseObject, error) {
	err := s.sourceSrv.DeleteSources(ctx)
	if err != nil {
		return server.DeleteSources500JSONResponse{}, nil
	}
	return apiServer.DeleteSources200JSONResponse{}, nil
}

// (DELETE /api/v1/sources/{id})
func (s *ServiceHandler) DeleteSource(ctx context.Context, request apiServer.DeleteSourceRequestObject) (apiServer.DeleteSourceResponseObject, error) {
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
func (s *ServiceHandler) GetSource(ctx context.Context, request apiServer.GetSourceRequestObject) (apiServer.GetSourceResponseObject, error) {
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

	return server.GetSource200JSONResponse(mappers.SourceToApi(*source)), nil
}

// (PUT /api/v1/sources/{id})
func (s *ServiceHandler) UpdateSource(ctx context.Context, request apiServer.UpdateSourceRequestObject) (apiServer.UpdateSourceResponseObject, error) {
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
			return apiServer.UpdateSource500JSONResponse{Message: fmt.Sprintf("failed to update source %s: %v", request.Id, err)}, nil
		}
	}

	return apiServer.UpdateSource200JSONResponse(mappers.SourceToApi(*updatedSource)), nil
}

// (PUT /api/v1/sources/{id}/inventory)
func (s *ServiceHandler) UpdateInventory(ctx context.Context, request apiServer.UpdateInventoryRequestObject) (apiServer.UpdateInventoryResponseObject, error) {
	if request.Body == nil {
		return server.UpdateInventory400JSONResponse{Message: "empty body"}, nil
	}

	source, err := s.sourceSrv.GetSource(ctx, request.Id)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.UpdateInventory404JSONResponse{Message: err.Error()}, nil
		default:
			return server.UpdateInventory500JSONResponse{}, nil
		}
	}

	user := auth.MustHaveUser(ctx)
	if user.Organization != source.OrgID || user.Username != source.Username {
		message := fmt.Sprintf("forbidden to update inventory for source %s by user %s with org_id %s", request.Id, user.Username, user.Organization)
		return server.UpdateInventory403JSONResponse{Message: message}, nil
	}

	data, err := json.Marshal(request.Body.Inventory)
	if err != nil {
		return apiServer.UpdateInventory500JSONResponse{Message: fmt.Sprintf("failed to update source inventory %s: %v", request.Id, err)}, nil
	}

	updatedSource, err := s.sourceSrv.UpdateInventory(ctx, srvMappers.InventoryUpdateForm{
		AgentID:   request.Body.AgentId,
		SourceID:  request.Id,
		Inventory: data,
		VCenterID: request.Body.Inventory.Vcenter.Id,
	})
	if err != nil {
		switch err.(type) {
		case *service.ErrInvalidVCenterID:
			return server.UpdateInventory400JSONResponse{Message: err.Error()}, nil
		default:
			return apiServer.UpdateInventory500JSONResponse{Message: fmt.Sprintf("failed to update source inventory %s: %v", request.Id, err)}, nil
		}
	}

	return apiServer.UpdateInventory200JSONResponse(mappers.SourceToApi(updatedSource)), nil
}

// (HEAD /api/v1/sources/{id}/image)
func (s *ServiceHandler) HeadImage(ctx context.Context, request apiServer.HeadImageRequestObject) (apiServer.HeadImageResponseObject, error) {
	return nil, nil
}

// (GET /api/v1/sources/{id}/image-url)
func (s *ServiceHandler) GetSourceDownloadURL(ctx context.Context, request apiServer.GetSourceDownloadURLRequestObject) (apiServer.GetSourceDownloadURLResponseObject, error) {
	url, expireAt, err := s.sourceSrv.GetSourceDownloadURL(ctx, request.Id)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return apiServer.GetSourceDownloadURL404JSONResponse{Message: err.Error()}, nil
		default:
			return apiServer.GetSourceDownloadURL400JSONResponse{}, nil // FIX: should be 500
		}
	}
	return apiServer.GetSourceDownloadURL200JSONResponse{Url: url, ExpiresAt: &expireAt}, nil
}

// (GET /health)
func (s *ServiceHandler) Health(ctx context.Context, request apiServer.HealthRequestObject) (apiServer.HealthResponseObject, error) {
	return apiServer.Health200Response{}, nil
}
