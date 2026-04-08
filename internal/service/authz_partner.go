package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

type AuthzPartnerService struct {
	inner       PartnerServicer
	accountsSvc *AccountsService
	store       store.Store
}

func NewAuthzPartnerService(inner PartnerServicer, accounts *AccountsService, s store.Store) PartnerServicer {
	return &AuthzPartnerService{inner: inner, accountsSvc: accounts, store: s}
}

func (a *AuthzPartnerService) ListPartners(ctx context.Context) (model.GroupList, error) {
	return a.inner.ListPartners(ctx)
}

func (a *AuthzPartnerService) ListRequests(ctx context.Context, user auth.User) (model.PartnerCustomerList, error) {
	return a.inner.ListRequests(ctx, user)
}

func (a *AuthzPartnerService) CreateRequest(ctx context.Context, user auth.User, partnerID string, pc model.PartnerCustomer) (*model.PartnerCustomer, error) {
	return a.inner.CreateRequest(ctx, user, partnerID, pc)
}

func (a *AuthzPartnerService) CancelRequest(ctx context.Context, user auth.User, requestID uuid.UUID) error {
	return a.inner.CancelRequest(ctx, user, requestID)
}

func (a *AuthzPartnerService) GetPartner(ctx context.Context, user auth.User, partnerID string) (model.Group, error) {
	return a.inner.GetPartner(ctx, user, partnerID)
}

func (a *AuthzPartnerService) LeavePartner(ctx context.Context, user auth.User, partnerID string) error {
	return a.inner.LeavePartner(ctx, user, partnerID)
}

func (a *AuthzPartnerService) ListCustomers(ctx context.Context, user auth.User) (model.PartnerCustomerList, error) {
	return a.inner.ListCustomers(ctx, user)
}

func (a *AuthzPartnerService) UpdateRequest(ctx context.Context, user auth.User, requestID uuid.UUID, req model.Request) (*model.PartnerCustomer, error) {
	identity, err := a.accountsSvc.GetIdentity(ctx, user)
	if err != nil {
		return nil, err
	}

	if identity.Kind != KindPartner || identity.GroupID == nil {
		return nil, NewErrInvalidRequest("only partners can update requests")
	}

	pc, err := a.store.Partner().Get(ctx, store.NewPartnerQueryFilter().ByID(requestID))
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return nil, NewErrResourceNotFound(requestID, "partner request")
		}
		return nil, err
	}

	if pc.PartnerID != *identity.GroupID {
		return nil, NewErrForbidden("partner request", requestID.String())
	}

	return a.inner.UpdateRequest(ctx, user, requestID, req)
}

func (a *AuthzPartnerService) RemoveCustomer(ctx context.Context, user auth.User, username string) error {
	return a.inner.RemoveCustomer(ctx, user, username)
}
