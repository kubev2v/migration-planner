package v1alpha1

import (
	"context"
	"fmt"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/api/server"
	apiServer "github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/handlers/v1alpha1/mappers"
	"github.com/kubev2v/migration-planner/internal/handlers/validator"
	"github.com/kubev2v/migration-planner/internal/service"
	srvMappers "github.com/kubev2v/migration-planner/internal/service/mappers"
	"io"
)

type ServiceHandler struct {
	sourceSrv     *service.SourceService
	shareTokenSrv *service.ShareTokenService
}

func NewServiceHandler(sourceService *service.SourceService, shareTokenService *service.ShareTokenService) *ServiceHandler {
	return &ServiceHandler{
		sourceSrv:     sourceService,
		shareTokenSrv: shareTokenService,
	}
}

// (GET /api/v1/sources)
func (s *ServiceHandler) ListSources(ctx context.Context, request apiServer.ListSourcesRequestObject) (apiServer.ListSourcesResponseObject, error) {
	user := auth.MustHaveUser(ctx)

	filter := service.NewSourceFilter(service.WithOrgID(user.Organization))

	includeDefault := request.Params.IncludeDefault
	if includeDefault != nil && *includeDefault {
		filter = filter.WithOption(service.WithDefaultInventory())
	}

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
	v := validator.NewValidator()
	v.Register(validator.NewSourceValidationRules()...)

	if err := v.Struct(form); err != nil {
		return apiServer.CreateSource400JSONResponse{Message: err.Error()}, nil
	}

	user := auth.MustHaveUser(ctx)
	sourceCreateForm := mappers.SourceFormApi(form)
	sourceCreateForm.Username = user.Username
	sourceCreateForm.OrgID = user.Organization
	sourceCreateForm.EmailDomain = user.EmailDomain

	source, err := s.sourceSrv.CreateSource(ctx, sourceCreateForm)
	if err != nil {
		return apiServer.CreateSource500JSONResponse{}, nil
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
		return server.DeleteSource404JSONResponse{}, nil
	}

	user := auth.MustHaveUser(ctx)
	if user.Organization != source.OrgID {
		return server.DeleteSource403JSONResponse{}, nil
	}

	if err := s.sourceSrv.DeleteSource(ctx, request.Id); err != nil {
		return server.DeleteSource500JSONResponse{}, nil
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
	if user.Organization != source.OrgID {
		return server.GetSource403JSONResponse{Message: fmt.Sprintf("forbidden access to source %q", request.Id)}, nil
	}

	return server.GetSource200JSONResponse(mappers.SourceToApi(*source)), nil
}

// (PUT /api/v1/sources/{id})
func (s *ServiceHandler) UpdateSource(ctx context.Context, request apiServer.UpdateSourceRequestObject) (apiServer.UpdateSourceResponseObject, error) {
	source, err := s.sourceSrv.GetSource(ctx, request.Id)
	if err != nil {
		return server.UpdateSource404JSONResponse{Message: err.Error()}, nil
	}

	user := auth.MustHaveUser(ctx)
	if user.Organization != source.OrgID {
		return server.UpdateSource403JSONResponse{Message: fmt.Sprintf("forbidden to update source %s by user with org_id %s", request.Id, user.Organization)}, nil
	}

	updateSource, err := s.sourceSrv.UpdateInventory(ctx, srvMappers.InventoryUpdateForm{
		AgentId:   request.Body.AgentId,
		SourceID:  request.Id,
		Inventory: request.Body.Inventory,
	})
	if err != nil {
		switch err.(type) {
		case *service.ErrInvalidVCenterID:
			return server.UpdateSource400JSONResponse{Message: err.Error()}, nil
		default:
			return apiServer.UpdateSource500JSONResponse{Message: fmt.Sprintf("failed to update source %s", request.Id)}, nil
		}
	}

	return apiServer.UpdateSource200JSONResponse(mappers.SourceToApi(updateSource)), nil
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

// (PUT /api/v1/sources/{id}/rvtools)
func (s *ServiceHandler) UploadRvtoolsFile(ctx context.Context, request apiServer.UploadRvtoolsFileRequestObject) (apiServer.UploadRvtoolsFileResponseObject, error) {
	multipartReader := request.Body

	for {
		part, err := multipartReader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return server.UploadRvtoolsFile400JSONResponse{
				Message: "Failed to read multipart data",
			}, nil
		}

		if part.FormName() == "file" {
			err := s.sourceSrv.UploadRvtoolsFile(ctx, request.Id, part)
			if err != nil {
				switch err.(type) {
				case *service.ErrExcelFileNotValid:
					return apiServer.UploadRvtoolsFile400JSONResponse{Message: err.Error()}, nil
				case *service.ErrResourceNotFound:
					return apiServer.UploadRvtoolsFile404JSONResponse{Message: err.Error()}, nil
				default:
					return server.UploadRvtoolsFile400JSONResponse{
						Message: "Failed to read uploaded file content",
					}, nil
				}
			}
			break
		}
	}
	return server.UploadRvtoolsFile200JSONResponse{}, nil
}

// (GET /health)
func (s *ServiceHandler) Health(ctx context.Context, request apiServer.HealthRequestObject) (apiServer.HealthResponseObject, error) {
	return apiServer.Health200Response{}, nil
}

// (POST /api/v1/sources/{id}/share-token)
func (s *ServiceHandler) CreateShareToken(ctx context.Context, request apiServer.CreateShareTokenRequestObject) (apiServer.CreateShareTokenResponseObject, error) {
	// Check if source exists and user has access
	source, err := s.sourceSrv.GetSource(ctx, request.Id)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return apiServer.CreateShareToken404JSONResponse{Message: err.Error()}, nil
		default:
			return apiServer.CreateShareToken500JSONResponse{Message: err.Error()}, nil
		}
	}

	user := auth.MustHaveUser(ctx)
	if user.Organization != source.OrgID {
		return apiServer.CreateShareToken403JSONResponse{Message: fmt.Sprintf("forbidden access to source %q", request.Id)}, nil
	}

	// Create or get existing share token
	shareToken, err := s.shareTokenSrv.CreateShareToken(ctx, request.Id)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return apiServer.CreateShareToken404JSONResponse{Message: err.Error()}, nil
		case *service.ErrSourceNoInventory:
			// Check if it's an inventory validation error
			return apiServer.CreateShareToken400JSONResponse{Message: err.Error()}, nil
		default:
			return apiServer.CreateShareToken500JSONResponse{Message: err.Error()}, nil
		}
	}

	return apiServer.CreateShareToken200JSONResponse(mappers.ShareTokenToApi(*shareToken)), nil
}

// (DELETE /api/v1/sources/{id}/share-token)
func (s *ServiceHandler) DeleteShareToken(ctx context.Context, request apiServer.DeleteShareTokenRequestObject) (apiServer.DeleteShareTokenResponseObject, error) {
	// Check if source exists and user has access
	source, err := s.sourceSrv.GetSource(ctx, request.Id)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return apiServer.DeleteShareToken404JSONResponse{Message: err.Error()}, nil
		default:
			return apiServer.DeleteShareToken500JSONResponse{Message: err.Error()}, nil
		}
	}

	user := auth.MustHaveUser(ctx)
	if user.Organization != source.OrgID {
		return apiServer.DeleteShareToken403JSONResponse{Message: fmt.Sprintf("forbidden access to source %q", request.Id)}, nil
	}

	// Delete share token
	err = s.shareTokenSrv.DeleteShareToken(ctx, request.Id)
	if err != nil {
		return apiServer.DeleteShareToken500JSONResponse{Message: err.Error()}, nil
	}

	return apiServer.DeleteShareToken200JSONResponse{
		Message: func() *string { s := "Share token deleted successfully"; return &s }(),
		Status:  func() *string { s := "Success"; return &s }(),
	}, nil
}

// (GET /api/v1/sources/shared/{token})
func (s *ServiceHandler) GetSharedSource(ctx context.Context, request apiServer.GetSharedSourceRequestObject) (apiServer.GetSharedSourceResponseObject, error) {
	// Get source by token (no authentication required)
	source, err := s.shareTokenSrv.GetSourceByToken(ctx, request.Token)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return apiServer.GetSharedSource404JSONResponse{Message: err.Error()}, nil
		default:
			return apiServer.GetSharedSource500JSONResponse{Message: err.Error()}, nil
		}
	}

	return apiServer.GetSharedSource200JSONResponse(mappers.SourceToApi(*source)), nil
}

// (GET /api/v1/sources/{id}/share-token)
func (s *ServiceHandler) GetShareToken(ctx context.Context, request apiServer.GetShareTokenRequestObject) (apiServer.GetShareTokenResponseObject, error) {
	// Check if source exists and user has access
	source, err := s.sourceSrv.GetSource(ctx, request.Id)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return apiServer.GetShareToken404JSONResponse{Message: err.Error()}, nil
		default:
			return apiServer.GetShareToken500JSONResponse{Message: err.Error()}, nil
		}
	}

	user := auth.MustHaveUser(ctx)
	if user.Organization != source.OrgID {
		return apiServer.GetShareToken403JSONResponse{Message: fmt.Sprintf("forbidden access to source %q", request.Id)}, nil
	}

	// Get share token for this source
	shareToken, err := s.shareTokenSrv.GetShareTokenBySourceID(ctx, request.Id)
	if err != nil {
		switch err.(type) {
		case *service.ErrResourceNotFound:
			return apiServer.GetShareToken404JSONResponse{Message: "share token not found"}, nil
		default:
			return apiServer.GetShareToken500JSONResponse{Message: err.Error()}, nil
		}
	}

	return apiServer.GetShareToken200JSONResponse(mappers.ShareTokenToApi(*shareToken)), nil
}

// (GET /api/v1/sources/share-tokens)
func (s *ServiceHandler) ListShareTokens(ctx context.Context, request apiServer.ListShareTokensRequestObject) (apiServer.ListShareTokensResponseObject, error) {
	user := auth.MustHaveUser(ctx)

	// List all share tokens for the user's organization
	shareTokens, err := s.shareTokenSrv.ListShareTokens(ctx, user.Organization)
	if err != nil {
		return apiServer.ListShareTokens500JSONResponse{Message: err.Error()}, nil
	}

	return apiServer.ListShareTokens200JSONResponse(mappers.ShareTokenListToApi(shareTokens)), nil
}
