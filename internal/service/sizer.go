package service

import (
	"context"
	"errors"
	"fmt"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
)

// SizerService handles odf sizer related operations.
type SizerService struct {
}

func NewSizerService() *SizerService {
	return &SizerService{}
}

func (s *SizerService) CalculateSizing(ctx context.Context, req *api.CalculateSizingJSONRequestBody) (*api.SizingResponse, error) {
	// TODO: Implement sizing calculations
	return nil, fmt.Errorf("sizing calculation not yet implemented")
}

func (s *SizerService) Health() error {
	// TODO: Implement sizing health
	return errors.New("not implemented")
}
