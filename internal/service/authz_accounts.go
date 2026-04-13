package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

type AuthzAccountsService struct {
	inner AccountsServicer
}

func NewAuthzAccountsService(inner AccountsServicer) AccountsServicer {
	return &AuthzAccountsService{inner: inner}
}

// Initialize is bootstrap-only — called from api_server.Run before HTTP serving starts.
// It intentionally skips authz because no authenticated user exists at boot time.
// Must not be called from request handlers; use innerAccountsSvc.Initialize directly.
func (a *AuthzAccountsService) Initialize(ctx context.Context, adminGroup AdminGroup) error {
	return a.inner.Initialize(ctx, adminGroup)
}

func (a *AuthzAccountsService) GetIdentity(ctx context.Context, authUser auth.User) (Identity, error) {
	return a.inner.GetIdentity(ctx, authUser)
}

func (a *AuthzAccountsService) ListGroups(ctx context.Context, filter *store.GroupQueryFilter) (model.GroupList, error) {
	if err := a.requireAdmin(ctx, "groups"); err != nil {
		return nil, err
	}
	return a.inner.ListGroups(ctx, filter)
}

func (a *AuthzAccountsService) GetGroup(ctx context.Context, id uuid.UUID) (model.Group, error) {
	if err := a.requireAdmin(ctx, "groups"); err != nil {
		return model.Group{}, err
	}
	return a.inner.GetGroup(ctx, id)
}

func (a *AuthzAccountsService) CreateGroup(ctx context.Context, group model.Group) (model.Group, error) {
	if err := a.requireAdmin(ctx, "groups"); err != nil {
		return model.Group{}, err
	}
	return a.inner.CreateGroup(ctx, group)
}

func (a *AuthzAccountsService) UpdateGroup(ctx context.Context, group model.Group) (model.Group, error) {
	if err := a.requireAdmin(ctx, "groups"); err != nil {
		return model.Group{}, err
	}
	return a.inner.UpdateGroup(ctx, group)
}

func (a *AuthzAccountsService) DeleteGroup(ctx context.Context, id uuid.UUID) error {
	if err := a.requireAdmin(ctx, "groups"); err != nil {
		return err
	}
	return a.inner.DeleteGroup(ctx, id)
}

func (a *AuthzAccountsService) GetMember(ctx context.Context, username string) (model.Member, error) {
	if err := a.requireAdmin(ctx, "members"); err != nil {
		return model.Member{}, err
	}
	return a.inner.GetMember(ctx, username)
}

func (a *AuthzAccountsService) ListGroupMembers(ctx context.Context, groupID uuid.UUID) (model.MemberList, error) {
	if err := a.requireAdmin(ctx, "members"); err != nil {
		return nil, err
	}
	return a.inner.ListGroupMembers(ctx, groupID)
}

func (a *AuthzAccountsService) CreateMember(ctx context.Context, member model.Member) (model.Member, error) {
	if err := a.requireAdmin(ctx, "members"); err != nil {
		return model.Member{}, err
	}
	return a.inner.CreateMember(ctx, member)
}

func (a *AuthzAccountsService) UpdateGroupMember(ctx context.Context, groupID uuid.UUID, username string, member model.Member) (model.Member, error) {
	if err := a.requireAdmin(ctx, "members"); err != nil {
		return model.Member{}, err
	}
	return a.inner.UpdateGroupMember(ctx, groupID, username, member)
}

func (a *AuthzAccountsService) RemoveGroupMember(ctx context.Context, groupID uuid.UUID, username string) error {
	if err := a.requireAdmin(ctx, "members"); err != nil {
		return err
	}
	return a.inner.RemoveGroupMember(ctx, groupID, username)
}

func (a *AuthzAccountsService) requireAdmin(ctx context.Context, resource string) error {
	user := auth.MustHaveUser(ctx)
	identity, err := a.inner.GetIdentity(ctx, user)
	if err != nil {
		return fmt.Errorf("authz: failed to get identity: %w", err)
	}
	if identity.Kind != KindAdmin {
		return NewErrForbidden(resource, user.Username)
	}
	return nil
}
