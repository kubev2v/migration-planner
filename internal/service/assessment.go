package service

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/opa"
	"github.com/kubev2v/migration-planner/internal/rvtools"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"go.uber.org/zap"
)

const (
	SourceTypeAgent     string = "agent"
	SourceTypeInventory string = "inventory"
	SourceTypeRvtools   string = "rvtools"
)

type AssessmentService struct {
	store        store.Store
	opaValidator *opa.Validator
}

func NewAssessmentService(store store.Store, opaValidator *opa.Validator) *AssessmentService {
	return &AssessmentService{store: store, opaValidator: opaValidator}
}

func (as *AssessmentService) ListAssessments(ctx context.Context, filter *AssessmentFilter) ([]model.Assessment, error) {
	storeFilter := store.NewAssessmentQueryFilter().WithOrgID(filter.OrgID)

	if filter.Source != "" {
		storeFilter = storeFilter.WithSourceType(filter.Source)
	}
	if filter.SourceID != "" {
		storeFilter = storeFilter.WithSourceID(filter.SourceID)
	}
	if filter.NameLike != "" {
		storeFilter = storeFilter.WithNameLike(filter.NameLike)
	}

	assessments, err := as.store.Assessment().List(ctx, storeFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to list assessments: %w", err)

	}

	if filter.IncludeDefault {
		defaultAssessment, err := as.store.Assessment().List(ctx, store.NewAssessmentQueryFilter().WithDefaultInventory(true))
		if err != nil {
			return nil, err
		}
		return append(assessments, defaultAssessment...), nil
	}

	return assessments, nil
}

func (as *AssessmentService) GetAssessment(ctx context.Context, id uuid.UUID) (*model.Assessment, error) {
	assessment, err := as.store.Assessment().Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return nil, NewErrAssessmentNotFound(id)
		}
		return nil, fmt.Errorf("failed to get assessment: %w", err)
	}
	return assessment, nil
}

func (as *AssessmentService) CreateAssessment(ctx context.Context, createForm mappers.AssessmentCreateForm) (*model.Assessment, error) {
	ctx, err := as.store.NewTransactionContext(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	assessment := createForm.ToModel()

	var inventory v1alpha1.Inventory
	switch assessment.SourceType {
	case SourceTypeAgent:
		// We are sure to have a sourceID here. it has been validaded in handler's layer.
		source, err := as.store.Source().Get(ctx, *assessment.SourceID)
		if err != nil {
			return nil, err
		}
		if source.OrgID != assessment.OrgID {
			return nil, NewErrAssessmentCreationForbidden(source.ID)
		}
		if source.Inventory == nil {
			return nil, NewErrSourceHasNoInventory(source.ID)
		}
		inventory = source.Inventory.Data
	case SourceTypeInventory:
		inventory = createForm.Inventory
	case SourceTypeRvtools:
		content, err := io.ReadAll(createForm.RVToolsFile)
		if err != nil {
			return nil, err
		}
		rvtoolInventory, err := rvtools.ParseRVTools(ctx, content, as.opaValidator)
		if err != nil {
			return nil, fmt.Errorf("Error parsing RVTools file: %v", err)
		}
		inventory = *rvtoolInventory
	}

	createdAssessment, err := as.store.Assessment().Create(ctx, assessment, inventory)
	if err != nil {
		_, _ = store.Rollback(ctx)
		return nil, fmt.Errorf("failed to create assessment: %w", err)
	}

	if _, err := store.Commit(ctx); err != nil {
		return nil, err
	}

	zap.S().Named("assessment_service").Infow("Created assessment with initial snapshot", "assessment_id", createdAssessment.ID)

	return createdAssessment, nil
}

func (as *AssessmentService) UpdateAssessment(ctx context.Context, id uuid.UUID, name *string) (*model.Assessment, error) {
	ctx, err := as.store.NewTransactionContext(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	// Check if assessment exists and user has access
	assessment, err := as.store.Assessment().Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return nil, NewErrAssessmentNotFound(id)
		}
		return nil, fmt.Errorf("failed to get assessment: %w", err)
	}

	// if assessment source is inventory or rvtools don't update the inventory. update the name only
	// per design only assessments with sourceID can have multiple snapshots
	if assessment.SourceID != nil {
		source, err := as.store.Source().Get(ctx, *assessment.SourceID)
		if err != nil {
			return nil, err
		}
		// Update assessment with new snapshot
		if _, err := as.store.Assessment().Update(ctx, id, name, &source.Inventory.Data); err != nil {
			return nil, fmt.Errorf("failed to update assessment: %w", err)
		}

		if _, err := store.Commit(ctx); err != nil {
			return nil, err
		}

		zap.S().Named("assessment_service").Infow("updated assessment %s with new snapshot", "assessment_id", id)

		return as.GetAssessment(ctx, id)

	}

	// Update assessment with new snapshot
	if _, err = as.store.Assessment().Update(ctx, id, name, nil); err != nil {
		return nil, fmt.Errorf("failed to update assessment: %w", err)
	}

	if _, err := store.Commit(ctx); err != nil {
		return nil, err
	}

	zap.S().Named("assessment_service").Infow("updated assessment", "assessment_id", id)

	return as.GetAssessment(ctx, id)
}

func (as *AssessmentService) DeleteAssessment(ctx context.Context, id uuid.UUID) error {
	// Check if assessment exists
	_, err := as.store.Assessment().Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return NewErrAssessmentNotFound(id)
		}
		return fmt.Errorf("failed to get assessment: %w", err)
	}

	if err := as.store.Assessment().Delete(ctx, id); err != nil {
		return fmt.Errorf("failed to delete assessment: %w", err)
	}

	zap.S().Named("assessment_service").Infof("Deleted assessment %s", id)
	return nil
}

// AssessmentFilter represents filtering options for listing assessments
type AssessmentFilter struct {
	OrgID          string
	Source         string
	SourceID       string
	NameLike       string
	Limit          int
	Offset         int
	IncludeDefault bool
}

func NewAssessmentFilter(orgID string) *AssessmentFilter {
	return &AssessmentFilter{
		OrgID: orgID,
	}
}

func (f *AssessmentFilter) WithSource(source string) *AssessmentFilter {
	f.Source = source
	return f
}

func (f *AssessmentFilter) WithSourceID(sourceID string) *AssessmentFilter {
	f.SourceID = sourceID
	return f
}

func (f *AssessmentFilter) WithNameLike(pattern string) *AssessmentFilter {
	f.NameLike = pattern
	return f
}

func (f *AssessmentFilter) WithLimit(limit int) *AssessmentFilter {
	f.Limit = limit
	return f
}

func (f *AssessmentFilter) WithOffset(offset int) *AssessmentFilter {
	f.Offset = offset
	return f
}

func (f *AssessmentFilter) WithDefaultInventory() *AssessmentFilter {
	f.IncludeDefault = true
	return f
}
