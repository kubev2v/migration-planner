package v1alpha1

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/handlers/v1alpha1/mappers"
	"github.com/kubev2v/migration-planner/internal/handlers/validator"
	"github.com/kubev2v/migration-planner/internal/service"
	srvMappers "github.com/kubev2v/migration-planner/internal/service/mappers"
)

type ServiceHandler struct {
	sourceSrv     *service.SourceService
	assessmentSrv *service.AssessmentService
	jobSrv        *service.JobService
	sizerSrv      *service.SizerService
	estimationSrv *service.EstimationService
}

func NewServiceHandler(
	sourceService *service.SourceService,
	a *service.AssessmentService,
	j *service.JobService,
	sizer *service.SizerService,
	estimation *service.EstimationService,
) *ServiceHandler {
	return &ServiceHandler{
		sourceSrv:     sourceService,
		assessmentSrv: a,
		jobSrv:        j,
		sizerSrv:      sizer,
		estimationSrv: estimation,
	}
}

// validateSourceData validates the source data using the source validation rules
func validateSourceData(data interface{}) error {
	v := validator.NewValidator()
	v.Register(validator.NewSourceValidationRules()...)
	return v.Struct(data)
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
		return server.CreateSource500JSONResponse{Message: fmt.Sprintf("failed to create source: %v", err)}, nil
	}

	response, err := mappers.SourceToApi(source)
	if err != nil {
		return server.CreateSource500JSONResponse{Message: fmt.Sprintf("failed to map source to api: %v", err)}, nil
	}

	return server.CreateSource201JSONResponse(response), nil
}

// (DELETE /api/v1/sources)
func (s *ServiceHandler) DeleteSources(ctx context.Context, request server.DeleteSourcesRequestObject) (server.DeleteSourcesResponseObject, error) {
	err := s.sourceSrv.DeleteSources(ctx)
	if err != nil {
		return server.DeleteSources500JSONResponse{}, nil
	}
	return server.DeleteSources200JSONResponse{}, nil
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
		return server.UpdateInventory500JSONResponse{Message: fmt.Sprintf("failed to update source inventory %s: %v", request.Id, err)}, nil
	}

	updatedSource, err := s.sourceSrv.UpdateInventory(ctx, srvMappers.InventoryUpdateForm{
		AgentID:   request.Body.AgentId,
		SourceID:  request.Id,
		Inventory: data,
		VCenterID: request.Body.Inventory.VcenterId,
	})
	if err != nil {
		switch err.(type) {
		case *service.ErrInvalidVCenterID:
			return server.UpdateInventory400JSONResponse{Message: err.Error()}, nil
		default:
			return server.UpdateInventory500JSONResponse{Message: fmt.Sprintf("failed to update source inventory %s: %v", request.Id, err)}, nil
		}
	}

	response, err := mappers.SourceToApi(updatedSource)
	if err != nil {
		return server.UpdateInventory500JSONResponse{Message: fmt.Sprintf("failed to map source to api: %v", err)}, nil
	}

	return server.UpdateInventory200JSONResponse(response), nil
}

// (HEAD /api/v1/sources/{id}/image)
func (s *ServiceHandler) HeadImage(ctx context.Context, request server.HeadImageRequestObject) (server.HeadImageResponseObject, error) {
	return nil, nil
}

// (GET /api/v1/sources/{id}/image-url)
func (s *ServiceHandler) GetSourceDownloadURL(ctx context.Context, request server.GetSourceDownloadURLRequestObject) (server.GetSourceDownloadURLResponseObject, error) {
	url, expireAt, err := s.sourceSrv.GetSourceDownloadURL(ctx, request.Id)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return server.GetSourceDownloadURL404JSONResponse{Message: err.Error()}, nil
		default:
			return server.GetSourceDownloadURL400JSONResponse{}, nil // FIX: should be 500
		}
	}
	return server.GetSourceDownloadURL200JSONResponse{Url: url, ExpiresAt: &expireAt}, nil
}

// (GET /health)
func (s *ServiceHandler) Health(ctx context.Context, request server.HealthRequestObject) (server.HealthResponseObject, error) {
	return server.Health200Response{}, nil
}
