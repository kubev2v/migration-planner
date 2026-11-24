package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/log"
)

type AssessmentEnhancementDataServicer interface {
	Save(ctx context.Context, data model.AssessmentEnhancementData) (*model.AssessmentEnhancementData, error)
	Get(ctx context.Context, assessmentID uuid.UUID) (*model.AssessmentEnhancementData, error)
}

type AssessmentEnhancementDataService struct {
	store  store.Store
	logger *log.StructuredLogger
}

func NewAssessmentEnhancementDataService(s store.Store) *AssessmentEnhancementDataService {
	return &AssessmentEnhancementDataService{
		store:  s,
		logger: log.NewDebugLogger("assessment_enhancement_data_service"),
	}
}

func (s *AssessmentEnhancementDataService) Save(
	ctx context.Context,
	data model.AssessmentEnhancementData,
) (*model.AssessmentEnhancementData, error) {
	result, err := s.store.AssessmentEnhancementData().Upsert(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("failed to save enhancement data: %w", err)
	}

	return result, nil
}

func (s *AssessmentEnhancementDataService) Get(
	ctx context.Context,
	assessmentID uuid.UUID,
) (*model.AssessmentEnhancementData, error) {
	input, err := s.store.AssessmentEnhancementData().Get(ctx, assessmentID)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return &model.AssessmentEnhancementData{}, nil
		}
		return nil, fmt.Errorf("failed to get enhancement data: %w", err)
	}

	return input, nil
}
