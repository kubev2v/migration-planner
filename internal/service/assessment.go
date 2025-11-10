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
	"github.com/kubev2v/migration-planner/pkg/log"
)

const (
	SourceTypeAgent     string = "agent"
	SourceTypeInventory string = "inventory"
	SourceTypeRvtools   string = "rvtools"
)

type AssessmentService struct {
	store        store.Store
	opaValidator *opa.Validator
	logger       *log.StructuredLogger
}

func NewAssessmentService(store store.Store, opaValidator *opa.Validator) *AssessmentService {
	return &AssessmentService{
		store:        store,
		opaValidator: opaValidator,
		logger:       log.NewDebugLogger("assessment_service"),
	}
}

func (as *AssessmentService) ListAssessments(ctx context.Context, filter *AssessmentFilter) ([]model.Assessment, error) {
	logger := as.logger.WithContext(ctx)
	tracer := logger.Operation("list_assessments").
		WithString("username", filter.Username).
		WithString("org_id", filter.OrgID).
		WithString("source", filter.Source).
		WithString("source_id", filter.SourceID).
		WithString("name_like", filter.NameLike).
		WithInt("limit", filter.Limit).
		WithInt("offset", filter.Offset).
		Build()

	storeFilter := store.NewAssessmentQueryFilter().WithUsername(filter.Username).WithOrgID(filter.OrgID)

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

	tracer.Success().WithInt("count", len(assessments)).Log()
	return assessments, nil
}

func (as *AssessmentService) GetAssessment(ctx context.Context, id uuid.UUID) (*model.Assessment, error) {
	logger := as.logger.WithContext(ctx)
	tracer := logger.Operation("get_assessment").
		WithUUID("assessment_id", id).
		Build()

	assessment, err := as.store.Assessment().Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return nil, NewErrAssessmentNotFound(id)
		}
		return nil, fmt.Errorf("failed to get assessment: %w", err)
	}

	tracer.Success().
		WithString("assessment_name", assessment.Name).
		WithString("source_type", assessment.SourceType).
		WithBool("has_source_id", assessment.SourceID != nil).
		Log()
	return assessment, nil
}

func (as *AssessmentService) CreateAssessment(ctx context.Context, createForm mappers.AssessmentCreateForm) (*model.Assessment, error) {
	logger := as.logger.WithContext(ctx)
	tracer := logger.Operation("create_assessment").
		WithString("org_id", createForm.OrgID).
		WithString("name", createForm.Name).
		WithString("source_type", createForm.Source).
		WithUUIDPtr("source_id", createForm.SourceID).
		Build()

	assessment := createForm.ToModel()
	tracer.Step("convert_form_to_model").WithUUID("assessment_id", assessment.ID).Log()

	var inventory v1alpha1.Inventory
	switch assessment.SourceType {
	case SourceTypeAgent:
		tracer.Step("process_agent_source").Log()
		// We are sure to have a sourceID here. it has been validaded in handler's layer.
		source, err := as.store.Source().Get(ctx, *assessment.SourceID)
		if err != nil {
			return nil, err
		}
		if source.OrgID != assessment.OrgID || source.Username != assessment.Username {
			return nil, NewErrAssessmentCreationForbidden(source.ID)
		}
		if source.Inventory == nil {
			return nil, NewErrSourceHasNoInventory(source.ID)
		}
		inventory = source.Inventory.Data
	case SourceTypeInventory:
		tracer.Step("process_inventory_source").Log()
		inventory = createForm.Inventory
	case SourceTypeRvtools:
		tracer.Step("process_rvtools_source").Log()
		content, err := io.ReadAll(createForm.RVToolsFile)
		if err != nil {
			return nil, err
		}
		tracer.Step("read_rvtools_file").WithInt("file_size", len(content)).Log()
		clusteredInventory, err := rvtools.ParseRVTools(ctx, content, as.opaValidator)
		if err != nil {
			return nil, NewErrRVToolsFileCorrupted(fmt.Sprintf("error parsing RVTools file: %v", err))
		}

		inventory = *clusteredInventory.VCenter
		tracer.Step("parsed_rvtools_inventory").Log()
	}

	ctx, err := as.store.NewTransactionContext(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	createdAssessment, err := as.store.Assessment().Create(ctx, assessment, inventory)
	if err != nil {
		_, _ = store.Rollback(ctx)

		if errors.Is(err, store.ErrDuplicateKey) {
			return nil, NewErrAssessmentDuplicateName(assessment.Name)
		}

		return nil, fmt.Errorf("failed to create assessment: %w", err)
	}

	tracer.Step("assessment_created_in_db").WithUUID("created_assessment_id", createdAssessment.ID).Log()

	if _, err := store.Commit(ctx); err != nil {
		return nil, err
	}

	tracer.Success().
		WithUUID("assessment_id", createdAssessment.ID).
		WithString("assessment_name", createdAssessment.Name).
		WithString("source_type", createdAssessment.SourceType).
		Log()

	return createdAssessment, nil
}

func (as *AssessmentService) UpdateAssessment(ctx context.Context, id uuid.UUID, name *string) (*model.Assessment, error) {
	logger := as.logger.WithContext(ctx)
	tracer := logger.Operation("update_assessment").
		WithUUID("assessment_id", id).
		WithStringPtr("new_name", name).
		Build()

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

	tracer.Step("assessment_exists").WithString("current_name", assessment.Name).WithBool("has_source_id", assessment.SourceID != nil).Log()

	// if assessment source is inventory or rvtools don't update the inventory. update the name only
	// per design only assessments with sourceID can have multiple snapshots
	if assessment.SourceID != nil {
		tracer.Step("updating_with_new_snapshot").WithUUIDPtr("source_id", assessment.SourceID).Log()
		source, err := as.store.Source().Get(ctx, *assessment.SourceID)
		if err != nil {
			return nil, err
		}
		tracer.Step("source_retrieved").WithUUID("source_id", source.ID).Log()
		// Update assessment with new snapshot
		if _, err := as.store.Assessment().Update(ctx, id, name, &source.Inventory.Data); err != nil {
			return nil, fmt.Errorf("failed to update assessment: %w", err)
		}

		if _, err := store.Commit(ctx); err != nil {
			return nil, err
		}

		tracer.Success().WithString("update_type", "with_new_snapshot").Log()
		return as.GetAssessment(ctx, id)
	}

	tracer.Step("updating_name_only").Log()
	// Update assessment with new snapshot
	if _, err = as.store.Assessment().Update(ctx, id, name, nil); err != nil {
		return nil, fmt.Errorf("failed to update assessment: %w", err)
	}

	if _, err := store.Commit(ctx); err != nil {
		return nil, err
	}

	tracer.Success().WithString("update_type", "name_only").Log()
	return as.GetAssessment(ctx, id)
}

func (as *AssessmentService) DeleteAssessment(ctx context.Context, id uuid.UUID) error {
	logger := as.logger.WithContext(ctx)
	tracer := logger.Operation("delete_assessment").
		WithUUID("assessment_id", id).
		Build()

	// Check if assessment exists
	assessment, err := as.store.Assessment().Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return NewErrAssessmentNotFound(id)
		}
		return fmt.Errorf("failed to get assessment: %w", err)
	}

	tracer.Step("assessment_exists_for_delete").
		WithString("assessment_name", assessment.Name).
		WithString("source_type", assessment.SourceType).
		WithBool("has_source_id", assessment.SourceID != nil).
		Log()

	if err := as.store.Assessment().Delete(ctx, id); err != nil {
		return fmt.Errorf("failed to delete assessment: %w", err)
	}

	tracer.Success().WithString("deleted_assessment_name", assessment.Name).Log()
	return nil
}

// AssessmentFilter represents filtering options for listing assessments
type AssessmentFilter struct {
	OrgID    string
	Username string
	Source   string
	SourceID string
	NameLike string
	Limit    int
	Offset   int
}

func NewAssessmentFilter(username, orgID string) *AssessmentFilter {
	return &AssessmentFilter{
		Username: username,
		OrgID:    orgID,
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
