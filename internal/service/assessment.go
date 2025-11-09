package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"encoding/json"

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
	riverClient  *river.Client[pgx.Tx]
	logger       *log.StructuredLogger
}

func NewAssessmentService(store store.Store, opaValidator *opa.Validator, riverClient *river.Client[pgx.Tx]) *AssessmentService {
	return &AssessmentService{
		store:        store,
		opaValidator: opaValidator,
		riverClient:  riverClient,
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

// CreateRvtoolsAssessment creates an assessment with RVTools processing
// It creates an empty assessment and snapshot, enqueues a River job, and returns immediately
func (as *AssessmentService) CreateRvtoolsAssessment(ctx context.Context, createForm mappers.AssessmentCreateForm) (*model.Assessment, error) {
	logger := as.logger.WithContext(ctx)
	tracer := logger.Operation("create_rvtools_assessment").
		WithString("org_id", createForm.OrgID).
		WithString("name", createForm.Name).
		Build()

	// RVTools processing requires River client (which requires PostgreSQL)
	if as.riverClient == nil {
		return nil, fmt.Errorf("river client is not available: RVTools processing requires PostgreSQL database with River job queue")
	}

	assessment := createForm.ToModel()
	tracer.Step("convert_form_to_model").WithUUID("assessment_id", assessment.ID).Log()

	// Validate file data
	if len(createForm.RVToolsFile) == 0 {
		return nil, fmt.Errorf("rvtools file is empty")
	}
	tracer.Step("validate_file").WithInt("file_size", len(createForm.RVToolsFile)).Log()

	// Encode to base64 for storage in job args
	encodedData := base64.StdEncoding.EncodeToString(createForm.RVToolsFile)
	tracer.Step("encoded_base64").WithInt("encoded_size", len(encodedData)).Log()

	// Start transaction
	var err error
	ctx, err = as.store.NewTransactionContext(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	// Create pending snapshot for async processing
	createdSnapshot := model.Snapshot{
		Status: model.SnapshotStatusPending,
	}

	// Create assessment with snapshot
	if err := as.store.Assessment().Create(ctx, &assessment, &createdSnapshot); err != nil {
		_, _ = store.Rollback(ctx)
		if errors.Is(err, store.ErrDuplicateKey) {
			return nil, NewErrAssessmentDuplicateName(assessment.Name)
		}
		return nil, fmt.Errorf("failed to create assessment: %w", err)
	}

	tracer.Step("assessment_and_snapshot_created").
		WithUUID("created_assessment_id", assessment.ID).
		WithInt("snapshot_id", int(createdSnapshot.ID)).
		WithString("snapshot_status", string(createdSnapshot.Status)).
		Log()

	// Enqueue River job for async processing (before committing transaction)
	// If this fails, we'll rollback and return error without creating assessment/snapshot
	_, err = as.riverClient.Insert(ctx, rvtools.RVToolsJobArgs{
		SnapshotID:   createdSnapshot.ID,
		AssessmentID: assessment.ID,
		RVToolsData:  encodedData,
	}, &river.InsertOpts{
		MaxAttempts: 3, // Retry up to 3 times on failure
		UniqueOpts: river.UniqueOpts{
			ByArgs: true, // Unique by assessment ID (from RVToolsJobArgs)
		},
	})
	if err != nil {
		_, _ = store.Rollback(ctx)
		tracer.Error(err).WithString("step", "enqueue_river_job").Log()
		return nil, fmt.Errorf("failed to enqueue RVTools processing job: %w", err)
	}

	tracer.Step("river_job_enqueued").Log()

	// Commit transaction after successful job enqueue
	ctx, err = store.Commit(ctx)
	if err != nil {
		return nil, err
	}

	// Load the created assessment with snapshots
	createdAssessment, err := as.store.Assessment().Get(ctx, assessment.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get created assessment: %w", err)
	}

	tracer.Success().
		WithUUID("assessment_id", createdAssessment.ID).
		WithString("assessment_name", createdAssessment.Name).
		WithInt("snapshot_id", int(createdSnapshot.ID)).
		Log()

	return createdAssessment, nil
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

	var inventory []byte
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
		if len(source.Inventory) == 0 {
			return nil, NewErrSourceHasNoInventory(source.ID)
		}
		inventory = source.Inventory
	case SourceTypeInventory:
		tracer.Step("process_inventory_source").Log()
		inventory = createForm.Inventory
	}

	ctx, err := as.store.NewTransactionContext(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	// Create ready snapshot with inventory
	createdSnapshot := model.Snapshot{
		Status:    model.SnapshotStatusReady,
		Inventory: inventory,
	}

	// Create assessment with snapshot
	if err := as.store.Assessment().Create(ctx, &assessment, &createdSnapshot); err != nil {
		_, _ = store.Rollback(ctx)
		if errors.Is(err, store.ErrDuplicateKey) {
			return nil, NewErrAssessmentDuplicateName(assessment.Name)
		}
		return nil, fmt.Errorf("failed to create assessment: %w", err)
	}

	tracer.Step("assessment_and_snapshot_created_in_db").
		WithUUID("created_assessment_id", assessment.ID).
		WithInt("snapshot_id", int(createdSnapshot.ID)).
		WithString("snapshot_status", string(createdSnapshot.Status)).
		Log()

	if _, err := store.Commit(ctx); err != nil {
		return nil, err
	}

	tracer.Success().
		WithUUID("assessment_id", assessment.ID).
		WithString("assessment_name", assessment.Name).
		WithString("source_type", assessment.SourceType).
		Log()

	// Load the created assessment with snapshots
	createdAssessment, err := as.store.Assessment().Get(ctx, assessment.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get created assessment: %w", err)
	}

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

	// Prepare update data
	updates := &model.Assessment{}
	if name != nil {
		updates.Name = *name
		tracer.Step("updating_name").WithString("new_name", *name).Log()
	}

	var newSnapshot *model.Snapshot

	// if assessment source is inventory or rvtools don't update the inventory. update the name only
	// per design only assessments with sourceID can have multiple snapshots
	if assessment.SourceID != nil {
		tracer.Step("adding_new_snapshot").WithUUIDPtr("source_id", assessment.SourceID).Log()
		source, err := as.store.Source().Get(ctx, *assessment.SourceID)
		if err != nil {
			return nil, err
		}
		tracer.Step("source_retrieved").WithUUID("source_id", source.ID).Log()

		// Create snapshot from source inventory
		var inventory v1alpha1.Inventory
		if err := json.Unmarshal(source.Inventory, &inventory); err != nil {
			return nil, fmt.Errorf("failed to unmarshal source inventory: %w", err)
		}

		newSnapshot = &model.Snapshot{
			Status:    model.SnapshotStatusReady,
			Inventory: source.Inventory,
		}

		// Update assessment with new snapshot
		if _, err := as.store.Assessment().Update(ctx, id, updates, newSnapshot); err != nil {
			return nil, fmt.Errorf("failed to update assessment: %w", err)
		}

		if _, err := store.Commit(ctx); err != nil {
			return nil, err
		}

		tracer.Success().WithString("update_type", "with_new_snapshot").Log()
		return as.GetAssessment(ctx, id)
	}

	tracer.Step("updating_name_only").Log()
	if _, err = as.store.Assessment().Update(ctx, id, updates, nil); err != nil {
		return nil, fmt.Errorf("failed to update assessment: %w", err)
	}

	if _, err := store.Commit(ctx); err != nil {
		return nil, err
	}

	tracer.Success().WithString("update_type", "name_only").Log()

	return as.GetAssessment(ctx, id)
}

func (as *AssessmentService) CancelJob(ctx context.Context, assessmentID uuid.UUID, orgID string) error {
	logger := as.logger.WithContext(ctx)
	tracer := logger.Operation("cancel_job").
		WithUUID("assessment_id", assessmentID).
		WithString("org_id", orgID).
		Build()

	// Get assessment to verify ownership
	assessment, err := as.store.Assessment().Get(ctx, assessmentID)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return NewErrAssessmentNotFound(assessmentID)
		}
		return err
	}

	if assessment.OrgID != orgID {
		return NewErrAssessmentNotFound(assessmentID) // Hide existence from unauthorized users
	}

	tracer.Step("assessment_verified").Log()

	// Find processing snapshot
	filter := store.NewSnapshotQueryFilter().
		WithAssessmentID(assessmentID.String()).
		WithStatuses([]string{
			string(model.SnapshotStatusPending),
			string(model.SnapshotStatusParsing),
			string(model.SnapshotStatusValidating),
		})

	snapshots, err := as.store.Snapshot().List(ctx, filter)
	if err != nil {
		return err
	}

	if len(snapshots) == 0 {
		return NewErrNoProcessingJob(assessmentID)
	}

	snapshot := &snapshots[0]

	// Only cancel if status is pending, parsing, or validating
	if snapshot.Status != model.SnapshotStatusPending &&
		snapshot.Status != model.SnapshotStatusParsing &&
		snapshot.Status != model.SnapshotStatusValidating {
		return NewErrJobCannotBeCancelled(snapshot.ID, string(snapshot.Status))
	}

	tracer.Step("snapshot_found").WithInt("snapshot_id", int(snapshot.ID)).WithString("status", string(snapshot.Status)).Log()

	// Cancel the River job before deleting the snapshot
	if as.riverClient != nil {
		jobID, err := as.store.RiverJob().GetJob(ctx, assessmentID)
		if err != nil {
			tracer.Error(err).WithString("step", "find_river_job").Log()
			return fmt.Errorf("failed to find river job: %w", err)
		}

		if jobID != nil {
			tracer.Step("found_river_job").WithInt("job_id", int(*jobID)).Log()

			// Cancel the job
			_, err = as.riverClient.JobCancel(ctx, *jobID)
			if err != nil {
				tracer.Error(err).WithString("step", "cancel_river_job").Log()
				// Continue even if cancellation fails - the job might already be completed/cancelled
			} else {
				tracer.Step("river_job_cancelled").Log()
			}
		} else {
			tracer.Step("no_active_river_job_found").Log()
		}
	} else {
		tracer.Step("river_client_unavailable_skipping_job_cancellation").Log()
	}

	// Start transaction
	ctx, err = as.store.NewTransactionContext(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	// Delete the snapshot
	if err := as.store.Snapshot().Delete(ctx, snapshot.ID); err != nil {
		_, _ = store.Rollback(ctx)
		return fmt.Errorf("failed to delete snapshot: %w", err)
	}

	tracer.Step("snapshot_deleted").Log()

	// Check if this was the only snapshot
	snapshotFilter := store.NewSnapshotQueryFilter().WithAssessmentID(assessmentID.String())
	remainingSnapshots, err := as.store.Snapshot().List(ctx, snapshotFilter)
	if err != nil {
		_, _ = store.Rollback(ctx)
		return err
	}

	// If no snapshots remain, delete the assessment
	if len(remainingSnapshots) == 0 {
		if err := as.store.Assessment().Delete(ctx, assessmentID); err != nil {
			_, _ = store.Rollback(ctx)
			return fmt.Errorf("failed to delete assessment: %w", err)
		}
		tracer.Step("assessment_deleted").Log()
	}

	// Commit transaction
	if _, err := store.Commit(ctx); err != nil {
		return err
	}

	tracer.Success().Log()
	return nil
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
