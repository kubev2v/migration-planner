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
	"github.com/kubev2v/migration-planner/pkg/events/kafka"
	"github.com/kubev2v/migration-planner/pkg/events/notification"
)

type EventAssessmentService struct {
	inner       service.AssessmentServicer
	store       store.Store
	outbox      *OutboxService
	accountsSvc service.AccountsServicer
}

func NewEventAssessmentService(inner service.AssessmentServicer, s store.Store, accountsSvc service.AccountsServicer) service.AssessmentServicer {
	return &EventAssessmentService{inner: inner, store: s, outbox: NewOutboxService(s), accountsSvc: accountsSvc}
}

func (e *EventAssessmentService) ListAssessments(ctx context.Context, filter *service.AssessmentFilter) ([]model.Assessment, error) {
	assessments, err := e.inner.ListAssessments(ctx, filter)
	if err != nil {
		return nil, err
	}

	payload := kafka.NewVisitorPayload(filter.Username, filter.OrgID)
	ceBytes, err := kafka.BuildCloudEvent(kafka.VisitorEventType, payload)
	if err != nil {
		return nil, err
	}
	if err := e.outbox.Insert(ctx, kafka.VisitorEventType, ceBytes); err != nil {
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

	payload := kafka.NewAssessmentCreatedPayload(kafka.AssessmentData{
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
	ceBytes, err := kafka.BuildCloudEvent(kafka.AssessmentCreatedEventType, payload)
	if err != nil {
		return nil, err
	}
	if err := e.outbox.Insert(ctx, kafka.AssessmentCreatedEventType, ceBytes); err != nil {
		return nil, err
	}

	// When a new assessment is created on behalf of a customer by a partner
	// notify the customer by firing an email notification
	if user, ok := auth.UserFromContext(ctx); ok {
		identity, err := e.accountsSvc.GetIdentity(ctx, user)
		if err != nil {
			return nil, err
		}
		if identity.Kind == service.KindPartner || identity.Kind == service.KindAdmin {
			notificationBytes, err := notification.Build(
				notification.AssessmentCreatedEventType,
				assessment.OrgID,
				notification.SeverityImportant,
				map[string]string{"assessment_id": assessment.ID.String()},
				nil,
				notification.Recipient{Users: []string{assessment.Username}, IgnoreUserPreferences: true},
			)
			if err != nil {
				return nil, err
			}
			if err := e.outbox.Insert(ctx, notification.AssessmentCreatedEventType, notificationBytes); err != nil {
				return nil, err
			}
		}
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

	deletedAt := time.Now().UTC()

	if err := e.inner.DeleteAssessment(ctx, id); err != nil {
		return err
	}

	payload := kafka.NewAssessmentDeletedPayload(assessment.ID.String(), deletedAt)
	ceBytes, err := kafka.BuildCloudEvent(kafka.AssessmentDeletedEventType, payload)
	if err != nil {
		return err
	}
	if err := e.outbox.Insert(ctx, kafka.AssessmentDeletedEventType, ceBytes); err != nil {
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

	if err := e.inner.ShareAssessment(ctx, id); err != nil {
		return err
	}

	identity, err := e.accountsSvc.GetIdentity(ctx, user)
	if err != nil {
		return err
	}

	// Because of ShareAssessment success Must be a consumer with a PartnerID
	partnerGID, err := uuid.Parse(*identity.PartnerID)
	if err != nil {
		return err
	}

	group, err := e.store.Accounts().GetGroup(ctx, partnerGID)
	if err != nil {
		return err
	}

	var notifiedUsers []string
	for _, m := range group.Members {
		notifiedUsers = append(notifiedUsers, m.Username)
	}

	payload := kafka.NewShareAssessmentPayload(user.Username, id.String(), *identity.PartnerID)
	ceBytes, err := kafka.BuildCloudEvent(kafka.ShareAssessmentEventType, payload)
	if err != nil {
		return err
	}
	if err := e.outbox.Insert(ctx, kafka.ShareAssessmentEventType, ceBytes); err != nil {
		return err
	}

	// Notify a partner when a customer shared an assessment with him
	notificationBytes, err := notification.Build(
		notification.AssessmentSharedEventType,
		group.Company,
		notification.SeverityImportant,
		map[string]string{"assessment_id": id.String()},
		nil,
		notification.Recipient{OnlyAdmins: true, IgnoreUserPreferences: true, Users: notifiedUsers},
	)
	if err != nil {
		return err
	}
	if err := e.outbox.Insert(ctx, notification.AssessmentSharedEventType, notificationBytes); err != nil {
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

	if err := e.inner.UnshareAssessment(ctx, id); err != nil {
		return err
	}

	payload := kafka.NewUnshareAssessmentPayload(user.Username, id.String())
	ceBytes, err := kafka.BuildCloudEvent(kafka.UnshareAssessmentEventType, payload)
	if err != nil {
		return err
	}
	if err := e.outbox.Insert(ctx, kafka.UnshareAssessmentEventType, ceBytes); err != nil {
		return err
	}

	if _, err := store.Commit(ctx); err != nil {
		return err
	}

	return nil
}
