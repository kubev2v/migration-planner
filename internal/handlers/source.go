package handlers

import (
	"context"

	"github.com/go-playground/validator/v10"
	apiServer "github.com/kubev2v/migration-planner/internal/api/server"
)

type ServiceHandler struct {
	validate *validator.Validate
}

func NewServiceHandler() *ServiceHandler {
	validator := validator.New()
	registerSourceCreateFormValidation(validator)

	return &ServiceHandler{
		validate: validator,
	}
}

// (GET /api/v1/sources)
func (s *ServiceHandler) ListSources(ctx context.Context, request apiServer.ListSourcesRequestObject) (apiServer.ListSourcesResponseObject, error) {
	return nil, nil
}

// (POST /api/v1/sources)
func (s *ServiceHandler) CreateSource(ctx context.Context, request apiServer.CreateSourceRequestObject) (apiServer.CreateSourceResponseObject, error) {
	return nil, nil
}

// (DELETE /api/v1/sources)
func (s *ServiceHandler) DeleteSources(ctx context.Context, request apiServer.DeleteSourcesRequestObject) (apiServer.DeleteSourcesResponseObject, error) {
	return nil, nil
}

// (DELETE /api/v1/sources/{id})
func (s *ServiceHandler) DeleteSource(ctx context.Context, request apiServer.DeleteSourceRequestObject) (apiServer.DeleteSourceResponseObject, error) {
	return nil, nil
}

// (GET /api/v1/sources/{id})
func (s *ServiceHandler) GetSource(ctx context.Context, request apiServer.GetSourceRequestObject) (apiServer.GetSourceResponseObject, error) {
	return nil, nil
}

// (PUT /api/v1/sources/{id})
func (s *ServiceHandler) UpdateSource(ctx context.Context, request apiServer.UpdateSourceRequestObject) (apiServer.UpdateSourceResponseObject, error) {
	return nil, nil
}

// (HEAD /api/v1/sources/{id}/image)
func (s *ServiceHandler) HeadImage(ctx context.Context, request apiServer.HeadImageRequestObject) (apiServer.HeadImageResponseObject, error) {
	return nil, nil
}

// (GET /api/v1/sources/{id}/image-url)
func (s *ServiceHandler) GetSourceDownloadURL(ctx context.Context, request apiServer.GetSourceDownloadURLRequestObject) (apiServer.GetSourceDownloadURLResponseObject, error) {
	return nil, nil
}

// (PUT /api/v1/sources/{id}/rvtools)
func (s *ServiceHandler) UploadRvtoolsFile(ctx context.Context, request apiServer.UploadRvtoolsFileRequestObject) (apiServer.UploadRvtoolsFileResponseObject, error) {
	return nil, nil
}

// (GET /health)
func (s *ServiceHandler) Health(ctx context.Context, request apiServer.HealthRequestObject) (apiServer.HealthResponseObject, error) {
	return nil, nil
}
