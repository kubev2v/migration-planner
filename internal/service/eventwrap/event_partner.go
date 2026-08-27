package eventwrap

import (
	"context"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"github.com/kubev2v/migration-planner/pkg/events"
	"github.com/kubev2v/migration-planner/pkg/events/kafka"
	"github.com/kubev2v/migration-planner/pkg/events/notification"
)

type EventPartnerService struct {
	inner  service.PartnerServicer
	store  store.Store
	outbox *OutboxService
}

func NewEventPartnerService(inner service.PartnerServicer, s store.Store) service.PartnerServicer {
	return &EventPartnerService{inner: inner, store: s, outbox: NewOutboxService(s)}
}

func (e *EventPartnerService) ListPartners(ctx context.Context) (model.GroupList, error) {
	return e.inner.ListPartners(ctx)
}

func (e *EventPartnerService) ListRequests(ctx context.Context, user auth.User) (model.PartnerCustomerList, error) {
	return e.inner.ListRequests(ctx, user)
}

func (e *EventPartnerService) CreateRequest(ctx context.Context, user auth.User, partnerID string, pc model.PartnerCustomer) (*model.PartnerCustomer, error) {
	created, err := e.inner.CreateRequest(ctx, user, partnerID, pc)
	if err != nil {
		return nil, err
	}

	payload := kafka.NewPartnerCustomerPayload(kafka.PartnerCustomerData{
		ID:               created.ID.String(),
		CustomerUsername: created.Username,
		PartnerID:        created.PartnerID,
		RequestStatus:    string(created.RequestStatus),
		Location:         created.Location,
		AcceptedAt:       created.AcceptedAt,
		TerminatedAt:     created.TerminatedAt,
		CreatedAt:        created.CreatedAt,
	})
	ceBytes, err := kafka.BuildCloudEvent(kafka.PartnerCustomerEventType, payload)
	if err != nil {
		return nil, err
	}
	if err := e.outbox.Insert(ctx, events.EventTypeKafka, ceBytes); err != nil {
		return nil, err
	}

	var notifiedUsers []string
	for _, m := range created.Partner.Members {
		notifiedUsers = append(notifiedUsers, m.Username)
	}

	if len(notifiedUsers) > 0 {
		// Notify the partner when a customer sent a partnership request
		notificationBytes, err := notification.Build(
			notification.PartnershipRequestEventType,
			"", // Todo: Send the correct console.redhat.com partner org_id
			notification.SeverityImportant,
			nil,
			notification.Recipient{IgnoreUserPreferences: true, Users: notifiedUsers},
		)
		if err != nil {
			return nil, err
		}
		if err := e.outbox.Insert(ctx, events.EventTypeNotification, notificationBytes); err != nil {
			return nil, err
		}
	}

	return created, nil
}

func (e *EventPartnerService) CancelRequest(ctx context.Context, user auth.User, requestID uuid.UUID) error {
	ctx, err := e.store.NewTransactionContext(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	if err := e.inner.CancelRequest(ctx, user, requestID); err != nil {
		return err
	}

	pc, err := e.store.PartnerCustomer().Get(ctx, store.NewPartnerQueryFilter().ByID(requestID))
	if err != nil {
		return err
	}

	payload := kafka.NewPartnerCustomerPayload(kafka.PartnerCustomerData{
		ID:               pc.ID.String(),
		CustomerUsername: pc.Username,
		PartnerID:        pc.PartnerID,
		RequestStatus:    string(pc.RequestStatus),
		Location:         pc.Location,
		AcceptedAt:       pc.AcceptedAt,
		TerminatedAt:     pc.TerminatedAt,
		CreatedAt:        pc.CreatedAt,
	})
	ceBytes, err := kafka.BuildCloudEvent(kafka.PartnerCustomerEventType, payload)
	if err != nil {
		return err
	}
	if err := e.outbox.Insert(ctx, events.EventTypeKafka, ceBytes); err != nil {
		return err
	}

	if _, err := store.Commit(ctx); err != nil {
		return err
	}

	return nil
}

func (e *EventPartnerService) GetPartner(ctx context.Context, user auth.User, partnerID string) (model.Group, error) {
	return e.inner.GetPartner(ctx, user, partnerID)
}

func (e *EventPartnerService) LeavePartner(ctx context.Context, user auth.User, partnerID string) error {
	ctx, err := e.store.NewTransactionContext(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	pc, _ := e.store.PartnerCustomer().Get(ctx, store.NewPartnerQueryFilter().ByUsername(user.Username).ByPartnerID(partnerID).ByStatus(model.RequestStatusAccepted))

	if err := e.inner.LeavePartner(ctx, user, partnerID); err != nil {
		return err
	}

	if pc != nil {
		refreshed, err := e.store.PartnerCustomer().Get(ctx, store.NewPartnerQueryFilter().ByID(pc.ID))
		if err != nil {
			return err
		}

		payload := kafka.NewPartnerCustomerPayload(kafka.PartnerCustomerData{
			ID:               refreshed.ID.String(),
			CustomerUsername: refreshed.Username,
			PartnerID:        refreshed.PartnerID,
			RequestStatus:    string(refreshed.RequestStatus),
			Location:         refreshed.Location,
			AcceptedAt:       refreshed.AcceptedAt,
			TerminatedAt:     refreshed.TerminatedAt,
			CreatedAt:        refreshed.CreatedAt,
		})
		ceBytes, err := kafka.BuildCloudEvent(kafka.PartnerCustomerEventType, payload)
		if err != nil {
			return err
		}
		if err := e.outbox.Insert(ctx, events.EventTypeKafka, ceBytes); err != nil {
			return err
		}
	}

	if _, err := store.Commit(ctx); err != nil {
		return err
	}

	return nil
}

func (e *EventPartnerService) ListCustomers(ctx context.Context, user auth.User) (model.PartnerCustomerList, error) {
	return e.inner.ListCustomers(ctx, user)
}

func (e *EventPartnerService) UpdateRequest(ctx context.Context, user auth.User, requestID uuid.UUID, req model.Request) (*model.PartnerCustomer, error) {
	updated, err := e.inner.UpdateRequest(ctx, user, requestID, req)
	if err != nil {
		return nil, err
	}

	payload := kafka.NewPartnerCustomerPayload(kafka.PartnerCustomerData{
		ID:               updated.ID.String(),
		CustomerUsername: updated.Username,
		PartnerID:        updated.PartnerID,
		RequestStatus:    string(updated.RequestStatus),
		Location:         updated.Location,
		AcceptedAt:       updated.AcceptedAt,
		TerminatedAt:     updated.TerminatedAt,
		CreatedAt:        updated.CreatedAt,
	})
	ceBytes, err := kafka.BuildCloudEvent(kafka.PartnerCustomerEventType, payload)
	if err != nil {
		return nil, err
	}
	if err := e.outbox.Insert(ctx, events.EventTypeKafka, ceBytes); err != nil {
		return nil, err
	}

	// Notify the customer when a partner accepted/rejected partnership request
	decision := "Accepted"
	if updated.RequestStatus == model.RequestStatusRejected {
		decision = "Declined"
	}
	reason := ""
	if updated.Reason != nil {
		reason = *updated.Reason
	}
	notificationBytes, err := notification.Build(
		notification.PartnershipResponseEventType,
		user.Organization,
		notification.SeverityImportant,
		map[string]string{"decision": decision, "reason": reason},
		notification.Recipient{Users: []string{updated.Username}, IgnoreUserPreferences: true},
	)
	if err != nil {
		return nil, err
	}
	if err := e.outbox.Insert(ctx, events.EventTypeNotification, notificationBytes); err != nil {
		return nil, err
	}

	return updated, nil
}

func (e *EventPartnerService) RemoveCustomer(ctx context.Context, user auth.User, username string) error {
	ctx, err := e.store.NewTransactionContext(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	if err := e.inner.RemoveCustomer(ctx, user, username); err != nil {
		return err
	}

	if _, err := store.Commit(ctx); err != nil {
		return err
	}

	return nil
}
