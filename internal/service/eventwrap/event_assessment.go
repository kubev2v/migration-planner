package eventwrap

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/events"
)

type EventAssessmentService struct {
	inner  service.AssessmentServicer
	store  store.Store
	outbox *OutboxService
}

func NewEventAssessmentService(inner service.AssessmentServicer, s store.Store) service.AssessmentServicer {
	return &EventAssessmentService{inner: inner, store: s, outbox: NewOutboxService(s)}
}

func (e *EventAssessmentService) ListAssessments(ctx context.Context, filter *service.AssessmentFilter) ([]model.Assessment, error) {
	assessments, err := e.inner.ListAssessments(ctx, filter)
	if err != nil {
		return nil, err
	}

	payload := events.NewVisitorPayload(filter.Username, filter.OrgID)
	ceBytes, err := events.BuildCloudEvent(events.VisitorEventType, payload)
	if err != nil {
		return nil, err
	}
	if err := e.outbox.Insert(ctx, events.VisitorEventType, ceBytes); err != nil {
		return nil, err
	}
	return assessments, nil
}

func (e *EventAssessmentService) GetAssessment(ctx context.Context, id uuid.UUID) (*model.Assessment, error) {
	return e.inner.GetAssessment(ctx, id)
}

func (e *EventAssessmentService) CreateAssessment(ctx context.Context, createForm mappers.AssessmentCreateForm) (*model.Assessment, error) {
	ctx, err := e.store.NewTransactionContext(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	assessment, err := e.inner.CreateAssessment(ctx, createForm)
	if err != nil {
		return nil, err
	}

	payload := events.NewAssessmentCreatedPayload(events.AssessmentData{
		ID:         assessment.ID.String(),
		SnapshotID: assessment.Snapshots[0].ID,
		Inventory:  assessment.Snapshots[0].Inventory,
		Name:       assessment.Name,
		OrgID:      assessment.OrgID,
		Username:   assessment.Username,
		SourceType: assessment.SourceType,
		CreatedAt:  assessment.CreatedAt,
		UpdatedAt:  assessment.UpdatedAt,
	})
	ceBytes, err := events.BuildCloudEvent(events.AssessmentCreatedEventType, payload)
	if err != nil {
		return nil, err
	}
	if err := e.outbox.Insert(ctx, events.AssessmentCreatedEventType, ceBytes); err != nil {
		return nil, err
	}

	if _, err := store.Commit(ctx); err != nil {
		return nil, err
	}

	return assessment, nil
}

func (e *EventAssessmentService) UpdateAssessment(ctx context.Context, id uuid.UUID, name *string) (*model.Assessment, error) {
	return e.inner.UpdateAssessment(ctx, id, name)
}

func (e *EventAssessmentService) DeleteAssessment(ctx context.Context, id uuid.UUID) error {
	ctx, err := e.store.NewTransactionContext(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	assessment, err := e.inner.GetAssessment(ctx, id)
	if err != nil {
		return err
	}

	if err := e.inner.DeleteAssessment(ctx, id); err != nil {
		return err
	}

	payload := events.NewAssessmentDeletedPayload(assessment.ID.String())
	ceBytes, err := events.BuildCloudEvent(events.AssessmentDeletedEventType, payload)
	if err != nil {
		return err
	}
	if err := e.outbox.Insert(ctx, events.AssessmentDeletedEventType, ceBytes); err != nil {
		return err
	}

	if _, err := store.Commit(ctx); err != nil {
		return err
	}

	return nil
}

func (e *EventAssessmentService) ShareAssessment(ctx context.Context, id uuid.UUID) error {
	user := auth.MustHaveUser(ctx)

	ctx, err := e.store.NewTransactionContext(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	assessment, err := e.inner.GetAssessment(ctx, id)
	if err != nil {
		return err
	}

	if err := e.inner.ShareAssessment(ctx, id); err != nil {
		return err
	}

	assessmentID := assessment.ID.String()
	payload := events.NewUserActionPayload(events.UserActionData{
		Username:     user.Username,
		AssessmentID: &assessmentID,
		Timestamp:    time.Now().UTC(),
	})
	ceBytes, err := events.BuildCloudEvent(events.ShareAssessmentEventType, payload)
	if err != nil {
		return err
	}
	if err := e.outbox.Insert(ctx, events.ShareAssessmentEventType, ceBytes); err != nil {
		return err
	}

	if _, err := store.Commit(ctx); err != nil {
		return err
	}

	return nil
}

func (e *EventAssessmentService) UnshareAssessment(ctx context.Context, id uuid.UUID) error {
	user := auth.MustHaveUser(ctx)

	ctx, err := e.store.NewTransactionContext(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	assessment, err := e.inner.GetAssessment(ctx, id)
	if err != nil {
		return err
	}

	if err := e.inner.UnshareAssessment(ctx, id); err != nil {
		return err
	}

	assessmentIDStr := assessment.ID.String()
	payload := events.NewUserActionPayload(events.UserActionData{
		Username:     user.Username,
		AssessmentID: &assessmentIDStr,
		Timestamp:    time.Now().UTC(),
	})
	ceBytes, err := events.BuildCloudEvent(events.UnshareAssessmentEventType, payload)
	if err != nil {
		return err
	}
	if err := e.outbox.Insert(ctx, events.UnshareAssessmentEventType, ceBytes); err != nil {
		return err
	}

	if _, err := store.Commit(ctx); err != nil {
		return err
	}

	return nil
}
