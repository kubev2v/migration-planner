package eventwrap

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/service/mappers"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/events/kafka"
	"github.com/kubev2v/migration-planner/pkg/events/notification"
	"github.com/kubev2v/migration-planner/pkg/integrations/iam"
)

type EventAssessmentService struct {
	inner       service.AssessmentServicer
	store       store.Store
	outbox      *OutboxService
	accountsSvc service.AccountsServicer
	iam         iam.Client
}

func NewEventAssessmentService(inner service.AssessmentServicer, s store.Store, accountsSvc service.AccountsServicer) *EventAssessmentService {
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

	// TODO: notify the customer when a partner creates an assessment on their behalf.
	// This is disabled because there is currently no "create on behalf of a customer"
	// mechanism: CreateAssessment always owns the assessment by the acting user
	// (assessment.OrgID/Username), and a partner can have multiple customers, so nothing
	// links a created assessment to a specific customer. As written this notified the
	// partner about their own assessment, not any customer. Re-enable once an on-behalf
	// create flow identifies the target customer.
	//
	// if user, ok := auth.UserFromContext(ctx); ok {
	// 	identity, err := e.accountsSvc.GetIdentity(ctx, user)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	if identity.Kind == service.KindPartner || identity.Kind == service.KindAdmin {
	// 		notificationBytes, err := notification.Build(
	// 			notification.AssessmentCreatedEventType,
	// 			assessment.OrgID,
	// 			notification.SeverityImportant,
	// 			map[string]string{"assessment_id": assessment.ID.String()},
	// 			notification.Recipient{Users: []string{assessment.Username}, IgnoreUserPreferences: true},
	// 		)
	// 		if err != nil {
	// 			return nil, err
	// 		}
	// 		if err := e.outbox.Insert(ctx, notification.AssessmentCreatedEventType, notificationBytes); err != nil {
	// 			return nil, err
	// 		}
	// 	}
	// }

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

	payload := kafka.NewShareAssessmentPayload(user.Username, id.String(), *identity.PartnerID)
	ceBytes, err := kafka.BuildCloudEvent(kafka.ShareAssessmentEventType, payload)
	if err != nil {
		return err
	}
	if err := e.outbox.Insert(ctx, kafka.ShareAssessmentEventType, ceBytes); err != nil {
		return err
	}

	// Because of ShareAssessment success Must be a consumer with a PartnerID
	partnerGID, err := uuid.Parse(*identity.PartnerID)
	if err != nil {
		return err
	}

	members, err := e.store.Accounts().ListMembers(ctx, store.NewMemberQueryFilter().ByGroupID(partnerGID))
	if err != nil {
		return err
	}

	usersByOrg := usersByOrgID(ctx, members, e.iam, e.store)
	if len(usersByOrg) == 0 {
		zap.S().Warnw("skipping assessment shared notification: no partner members with a known org_id",
			"partner_id", *identity.PartnerID)
	}

	for orgID, users := range usersByOrg {
		notificationBytes, err := notification.Build(
			notification.AssessmentSharedEventType,
			orgID,
			notification.SeverityImportant,
			map[string]string{"assessment_id": id.String()},
			notification.Recipient{IgnoreUserPreferences: true, Users: users},
		)
		if err != nil {
			return fmt.Errorf("failed to build notification for shared assessment event: %w", err)
		}
		if err := e.outbox.Insert(ctx, notification.AssessmentSharedEventType, notificationBytes); err != nil {
			return err
		}
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

func (e *EventAssessmentService) WithIamClient(client iam.Client) *EventAssessmentService {
	e.iam = client
	return e
}
