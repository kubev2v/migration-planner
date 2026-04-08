package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

type PartnerServicer interface {
	// Regular user
	ListPartners(ctx context.Context) (model.GroupList, error)
	ListRequests(ctx context.Context, user auth.User) (model.PartnerCustomerList, error)
	CreateRequest(ctx context.Context, user auth.User, partnerID string, pc model.PartnerCustomer) (*model.PartnerCustomer, error)
	CancelRequest(ctx context.Context, user auth.User, requestID uuid.UUID) error

	// Customer
	GetPartner(ctx context.Context, user auth.User, partnerID string) (model.Group, error)
	LeavePartner(ctx context.Context, user auth.User, partnerID string) error

	// Partner
	ListCustomers(ctx context.Context, user auth.User) (model.PartnerCustomerList, error)
	UpdateRequest(ctx context.Context, user auth.User, requestID uuid.UUID, req model.Request) (*model.PartnerCustomer, error)
	RemoveCustomer(ctx context.Context, user auth.User, username string) error
}

type PartnerService struct {
	store       store.Store
	accountsSvc *AccountsService
}

func NewPartnerService(store store.Store, accounts *AccountsService) PartnerServicer {
	return &PartnerService{store: store, accountsSvc: accounts}
}

// ListPartners returns all partner groups.
func (s *PartnerService) ListPartners(ctx context.Context) (model.GroupList, error) {
	return s.store.Accounts().ListGroups(ctx, store.NewGroupQueryFilter().ByKind("partner"))
}

// ListRequests returns all partner requests for a user.
func (s *PartnerService) ListRequests(ctx context.Context, user auth.User) (model.PartnerCustomerList, error) {
	return s.store.PartnerCustomer().List(ctx, store.NewPartnerQueryFilter().ByUsername(user.Username))
}

// CreateRequest creates a new partner request.
// Returns ErrInvalidRequest if the user is not a regular user.
// Returns ErrActiveRequestExists if the user already has a pending or accepted request.
func (s *PartnerService) CreateRequest(ctx context.Context, user auth.User, partnerID string, pc model.PartnerCustomer) (*model.PartnerCustomer, error) {
	existing, err := s.store.PartnerCustomer().List(ctx, store.NewPartnerQueryFilter().ByUsername(user.Username))
	if err != nil {
		return nil, err
	}

	for _, e := range existing {
		if e.RequestStatus == model.RequestStatusPending || e.RequestStatus == model.RequestStatusAccepted {
			return nil, NewErrActiveRequestExists(user.Username)
		}
	}

	// Verify the target partner group exists
	groupID, err := uuid.Parse(partnerID)
	if err != nil {
		return nil, NewErrResourceNotFoundByStr(partnerID, "partner")
	}
	group, err := s.store.Accounts().GetGroup(ctx, groupID)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return nil, NewErrResourceNotFoundByStr(partnerID, "partner")
		}
		return nil, err
	}
	if group.Kind != "partner" {
		return nil, NewErrResourceNotFoundByStr(partnerID, "partner")
	}

	pc.ID = uuid.New()
	pc.Username = user.Username
	pc.PartnerID = partnerID
	pc.RequestStatus = model.RequestStatusPending
	created, err := s.store.PartnerCustomer().Create(ctx, pc)
	if err != nil {
		if errors.Is(err, store.ErrDuplicateKey) {
			return nil, NewErrActiveRequestExists(user.Username)
		}
		return nil, err
	}
	return created, nil
}

// CancelRequest cancels a pending partner request.
func (s *PartnerService) CancelRequest(ctx context.Context, user auth.User, requestID uuid.UUID) error {
	pc, err := s.store.PartnerCustomer().Get(ctx, store.NewPartnerQueryFilter().ByID(requestID))
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return NewErrResourceNotFound(requestID, "partner request")
		}
		return err
	}
	if pc.Username != user.Username {
		return NewErrResourceNotFound(requestID, "partner request")
	}
	if pc.RequestStatus != model.RequestStatusPending {
		return NewErrInvalidRequest("only pending requests can be cancelled")
	}
	now := time.Now()
	_, err = s.store.PartnerCustomer().Update(ctx, model.PartnerCustomer{
		ID:            requestID,
		RequestStatus: model.RequestStatusCancelled,
		TerminatedAt:  &now,
	})
	return err
}

// GetPartner returns the partner group for a customer.
func (s *PartnerService) GetPartner(ctx context.Context, user auth.User, partnerID string) (model.Group, error) {
	// Verify the user is actually a customer of this partner
	pc, err := s.store.PartnerCustomer().Get(ctx, store.NewPartnerQueryFilter().ByUsername(user.Username).ByPartnerID(partnerID).ByStatus(model.RequestStatusAccepted))
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return model.Group{}, NewErrResourceNotFoundByStr(partnerID, "partner")
		}
		return model.Group{}, err
	}

	groupID, err := uuid.Parse(pc.PartnerID)
	if err != nil {
		return model.Group{}, NewErrResourceNotFoundByStr(partnerID, "partner")
	}
	group, err := s.store.Accounts().GetGroup(ctx, groupID)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return model.Group{}, NewErrResourceNotFoundByStr(partnerID, "partner")
		}
		return model.Group{}, err
	}
	return group, nil
}

// LeavePartner removes the customer relationship with a partner.
func (s *PartnerService) LeavePartner(ctx context.Context, user auth.User, partnerID string) error {
	pc, err := s.store.PartnerCustomer().Get(ctx, store.NewPartnerQueryFilter().ByUsername(user.Username).ByPartnerID(partnerID).ByStatus(model.RequestStatusAccepted))
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return NewErrResourceNotFoundByStr(partnerID, "partner")
		}
		return err
	}
	now := time.Now()
	_, err = s.store.PartnerCustomer().Update(ctx, model.PartnerCustomer{
		ID:            pc.ID,
		RequestStatus: model.RequestStatusCancelled,
		TerminatedAt:  &now,
	})
	return err
}

// ListCustomers returns all customer requests for the partner's group.
func (s *PartnerService) ListCustomers(ctx context.Context, user auth.User) (model.PartnerCustomerList, error) {
	identity, err := s.accountsSvc.GetIdentity(ctx, user)
	if err != nil {
		return nil, err
	}
	return s.store.PartnerCustomer().List(ctx, store.NewPartnerQueryFilter().ByPartnerID(*identity.GroupID))
}

// UpdateRequest accepts or rejects a customer request.
func (s *PartnerService) UpdateRequest(ctx context.Context, user auth.User, requestID uuid.UUID, req model.Request) (*model.PartnerCustomer, error) {
	pc, err := s.store.PartnerCustomer().Get(ctx, store.NewPartnerQueryFilter().ByID(requestID))
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return nil, NewErrResourceNotFound(requestID, "partner request")
		}
		return nil, err
	}

	if req.Status == model.RequestStatusRejected && req.Reason == "" {
		return nil, NewErrInvalidRequest("reason is required when rejecting a request")
	}

	if pc.RequestStatus != model.RequestStatusPending {
		return nil, NewErrInvalidRequest("only pending requests can be updated")
	}

	var reason *string
	if req.Reason != "" {
		reason = &req.Reason
	}

	update := model.PartnerCustomer{
		ID:            pc.ID,
		RequestStatus: req.Status,
		Reason:        reason,
	}
	if req.Status == model.RequestStatusAccepted {
		now := time.Now()
		update.AcceptedAt = &now
	}
	return s.store.PartnerCustomer().Update(ctx, update)
}

// RemoveCustomer removes a customer from the partner's group.
func (s *PartnerService) RemoveCustomer(ctx context.Context, user auth.User, username string) error {
	identity, err := s.accountsSvc.GetIdentity(ctx, user)
	if err != nil {
		return err
	}
	pc, err := s.store.PartnerCustomer().Get(ctx, store.NewPartnerQueryFilter().ByUsername(username).ByPartnerID(*identity.GroupID).ByStatus(model.RequestStatusAccepted))
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return NewErrResourceNotFoundByStr(username, "partner customer")
		}
		return err
	}
	now := time.Now()
	_, err = s.store.PartnerCustomer().Update(ctx, model.PartnerCustomer{
		ID:            pc.ID,
		RequestStatus: model.RequestStatusCancelled,
		TerminatedAt:  &now,
	})
	return err
}
