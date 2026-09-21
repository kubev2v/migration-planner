package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/kubev2v/migration-planner/pkg/opa"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/log"
)

type AssessmentServicer interface {
	ListAssessments(ctx context.Context, filter *AssessmentFilter) ([]model.Assessment, error)
	GetAssessment(ctx context.Context, id uuid.UUID) (*model.Assessment, error)
	CreateAssessment(ctx context.Context, createForm mappers.AssessmentCreateForm) (*model.Assessment, error)
	UpdateAssessment(ctx context.Context, id uuid.UUID, name *string) (*model.Assessment, error)
	DeleteAssessment(ctx context.Context, id uuid.UUID) error
	ShareAssessment(ctx context.Context, id uuid.UUID) error
	UnshareAssessment(ctx context.Context, id uuid.UUID) error
}

const (
	SourceTypeAgent     string = "agent"
	SourceTypeInventory string = "inventory"
	SourceTypeRvtools   string = "rvtools"
)

type AssessmentService struct {
	store        store.Store
	opaValidator *opa.Validator
	accountsSvc  *AccountsService
	logger       *log.StructuredLogger
}

func NewAssessmentService(store store.Store, opaValidator *opa.Validator, accountsSvc *AccountsService) *AssessmentService {
	return &AssessmentService{
		store:        store,
		opaValidator: opaValidator,
		accountsSvc:  accountsSvc,
		logger:       log.NewDebugLogger("assessment_service"),
	}
}

func (as *AssessmentService) ListAssessments(ctx context.Context, filter *AssessmentFilter) ([]model.Assessment, error) {
	user := auth.MustHaveUser(ctx)

	logger := as.logger.WithContext(ctx)
	tracer := logger.Operation("list_assessments").
		WithString("username", user.Username).
		WithString("org_id", user.Organization).
		WithString("source", filter.Source).
		WithString("source_id", filter.SourceID).
		WithString("name_like", filter.NameLike).
		WithInt("limit", filter.Limit).
		WithInt("offset", filter.Offset).
		Build()

	storeFilter := store.NewAssessmentQueryFilter()

	if len(filter.IDs) > 0 {
		storeFilter = storeFilter.WithIDs(filter.IDs)
	}
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
		WithString("source_type", createForm.SourceType).
		WithUUIDPtr("source_id", createForm.SourceID).
		Build()

	assessment := createForm.ToModel()
	tracer.Step("convert_form_to_model").WithUUID("assessment_id", assessment.ID).Log()

	var assessmentInventories []model.AssessmentInventory
	var subsetInventories []model.AssessmentSubsetInventory

	switch createForm.SourceType {
	case SourceTypeRvtools, SourceTypeInventory:
		tracer.Step("using_provided_inventories").WithInt("count", len(createForm.Inventories)).Log()
		for i := range createForm.Inventories {
			createForm.Inventories[i].AssessmentID = assessment.ID
		}
		assessmentInventories = createForm.Inventories

	case "agent":
		tracer.Step("process_source").Log()
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

		mainInv, err := model.NewAssessmentInventory(uuid.New(), assessment.Name, source.Inventory)
		if err != nil {
			return nil, fmt.Errorf("failed to read source inventory: %w", err)
		}

		// TODO: move this validation to the source handler when inventory is pushed by the agent
		if mainInv.VMsCount == 0 {
			return nil, NewErrInventoryHasNoVMs()
		}

		mainInv.AssessmentID = assessment.ID
		assessmentInventories = append(assessmentInventories, mainInv)

		sourceSubsetFilter := store.NewSourceSubsetInventoryQueryFilter().BySourceID(*assessment.SourceID)
		sourceSubsets, err := as.store.SourceSubsetInventory().List(ctx, sourceSubsetFilter)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch source subset inventories: %w", err)
		}
		tracer.Step("found_source_subsets").WithInt("count", len(sourceSubsets)).Log()

		for _, ss := range sourceSubsets {
			inv, err := model.NewAssessmentInventory(uuid.New(), ss.Name, ss.Inventory)
			if err != nil {
				return nil, fmt.Errorf("failed to read subset inventory %q: %w", ss.Name, err)
			}
			inv.IsSubset = true
			inv.AssessmentID = assessment.ID
			assessmentInventories = append(assessmentInventories, inv)

			subsetInventories = append(subsetInventories, model.AssessmentSubsetInventory{
				ID: uuid.New(), Name: ss.Name, VCenterID: ss.VCenterID,
				VMsCount: ss.VMsCount, Inventory: ss.Inventory,
			})
		}

	default:
		return nil, fmt.Errorf("assessment must have either inventories or a source ID")
	}

	// Build snapshot for backward compatibility
	for _, inv := range assessmentInventories {
		if !inv.IsSubset {
			assessment.Snapshots = []model.Snapshot{{
				Inventory:         inv.Inventory,
				Version:           inv.Version,
				SubsetInventories: subsetInventories,
			}}
			break
		}
	}

	assessment.Inventories = assessmentInventories

	ctx, err := as.store.NewTransactionContext(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	createdAssessment, err := as.store.Assessment().Create(ctx, assessment)
	if err != nil {
		if errors.Is(err, store.ErrDuplicateKey) {
			return nil, NewErrAssessmentDuplicateName(assessment.Name)
		}
		return nil, fmt.Errorf("failed to create assessment: %w", err)
	}

	tracer.Step("assessment_created_in_db").
		WithUUID("created_assessment_id", createdAssessment.ID).
		WithInt("inventory_count", len(assessmentInventories)).
		Log()

	if _, err := store.Commit(ctx); err != nil {
		return nil, err
	}

	as.store.RequestMetricsCacheRefresh()

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

	assessment, err := as.store.Assessment().Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return nil, NewErrAssessmentNotFound(id)
		}
		return nil, fmt.Errorf("failed to get assessment: %w", err)
	}

	tracer.Step("assessment_exists").WithString("current_name", assessment.Name).WithBool("has_source_id", assessment.SourceID != nil).Log()

	if assessment.SourceID != nil {
		tracer.Step("updating_with_new_inventory").WithUUIDPtr("source_id", assessment.SourceID).Log()
		source, err := as.store.Source().Get(ctx, *assessment.SourceID)
		if err != nil {
			return nil, err
		}
		tracer.Step("source_retrieved").WithUUID("source_id", source.ID).Log()

		var inventories []model.AssessmentInventory
		if source.Inventory != nil {
			inv, err := model.NewAssessmentInventory(uuid.New(), assessment.Name, source.Inventory)
			if err != nil {
				return nil, fmt.Errorf("failed to read source inventory: %w", err)
			}
			inv.AssessmentID = id
			inventories = append(inventories, inv)
		}

		if _, err := as.store.Assessment().Update(ctx, id, name, inventories); err != nil {
			return nil, fmt.Errorf("failed to update assessment: %w", err)
		}

		if _, err := store.Commit(ctx); err != nil {
			return nil, err
		}

		as.store.RequestMetricsCacheRefresh()

		tracer.Success().WithString("update_type", "with_new_inventory").Log()
		return as.GetAssessment(ctx, id)
	}

	tracer.Step("updating_name_only").Log()
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
	as.store.RequestMetricsCacheRefresh()

	tracer.Success().WithString("deleted_assessment_name", assessment.Name).Log()
	return nil
}

func (as *AssessmentService) ShareAssessment(ctx context.Context, id uuid.UUID) error {
	user := auth.MustHaveUser(ctx)

	ctx, err := as.store.NewTransactionContext(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	// Verify assessment exists
	if _, err := as.store.Assessment().Get(ctx, id); err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return NewErrAssessmentNotFound(id)
		}
		return err
	}

	// Resolve identity — must be a customer with a partner
	identity, err := as.accountsSvc.GetIdentity(ctx, user)
	if err != nil {
		return err
	}
	if identity.Kind != KindCustomer || identity.PartnerID == nil {
		return NewErrNotACustomer(user.Username)
	}

	// Write viewer relation: assessment:id#viewer@org:partnerID
	updates := store.NewRelationshipBuilder().
		With(model.NewAssessmentResource(id.String()), model.ViewerRelation, model.NewOrgSubject(*identity.PartnerID)).
		Build()

	if err := as.store.Authz().WriteRelationships(ctx, updates); err != nil {
		return fmt.Errorf("failed to share assessment: %w", err)
	}

	if _, err := store.Commit(ctx); err != nil {
		return err
	}

	return nil
}

func (as *AssessmentService) UnshareAssessment(ctx context.Context, id uuid.UUID) error {
	user := auth.MustHaveUser(ctx)

	ctx, err := as.store.NewTransactionContext(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	// Verify assessment exists
	if _, err := as.store.Assessment().Get(ctx, id); err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return NewErrAssessmentNotFound(id)
		}
		return err
	}

	// Resolve identity — must be a customer with a partner
	identity, err := as.accountsSvc.GetIdentity(ctx, user)
	if err != nil {
		return err
	}
	if identity.Kind != KindCustomer || identity.PartnerID == nil {
		return NewErrNotACustomer(user.Username)
	}

	// Delete viewer relation: assessment:id#viewer@org:partnerID
	updates := store.NewRelationshipBuilder().
		Without(model.NewAssessmentResource(id.String()), model.ViewerRelation, model.NewOrgSubject(*identity.PartnerID)).
		Build()

	if err := as.store.Authz().WriteRelationships(ctx, updates); err != nil {
		return fmt.Errorf("failed to unshare assessment: %w", err)
	}

	if _, err := store.Commit(ctx); err != nil {
		return err
	}

	return nil
}

// AssessmentFilter represents filtering options for listing assessments
type AssessmentFilter struct {
	Source   string
	SourceID string
	NameLike string
	IDs      []uuid.UUID
	Limit    int
	Offset   int
}

func NewAssessmentFilter() *AssessmentFilter {
	return &AssessmentFilter{}
}

func (f *AssessmentFilter) WithIDs(IDs []uuid.UUID) *AssessmentFilter {
	f.IDs = IDs
	return f
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
