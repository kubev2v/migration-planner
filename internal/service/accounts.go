package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
)

type AccountsService struct {
	store store.Store
}

func NewAccountsService(store store.Store) *AccountsService {
	return &AccountsService{store: store}
}

// Identity

type IdentityKind string

const (
	KindRegular  IdentityKind = "regular"
	KindCustomer IdentityKind = "customer"
	KindPartner  IdentityKind = "partner"
	KindAdmin    IdentityKind = "admin"
)

type Identity struct {
	Username  string
	Kind      IdentityKind
	GroupID   *string
	PartnerID *string
}

func (s *AccountsService) IsKind(ctx context.Context, user auth.User, kind IdentityKind) (bool, error) {
	identity, err := s.GetIdentity(ctx, user)
	if err != nil {
		return false, err
	}
	return identity.Kind == kind, nil
}

func (s *AccountsService) GetIdentity(ctx context.Context, authUser auth.User) (Identity, error) {
	// 1. Customer: accepted partner request
	partners, err := s.store.Partner().List(ctx,
		store.NewPartnerQueryFilter().ByUsername(authUser.Username).ByStatus(model.RequestStatusAccepted))
	if err != nil {
		return Identity{}, err
	}
	if len(partners) > 0 {
		partnerID := partners[0].PartnerID
		return Identity{Username: authUser.Username, Kind: KindCustomer, PartnerID: &partnerID}, nil
	}

	// 2. Member-based roles: partner / admin
	member, err := s.store.Accounts().GetMember(ctx, authUser.Username)
	if err != nil && !errors.Is(err, store.ErrRecordNotFound) {
		return Identity{}, err
	}

	if member.Group != nil {
		groupID := member.GroupID.String()
		switch member.Group.Kind {
		case "partner":
			return Identity{Username: member.Username, Kind: KindPartner, GroupID: &groupID}, nil
		case "admin":
			return Identity{Username: member.Username, Kind: KindAdmin, GroupID: &groupID}, nil
		}
	}

	// 3. Regular
	return Identity{Username: authUser.Username, Kind: KindRegular}, nil
}

func (s *AccountsService) ListGroups(ctx context.Context, filter *store.GroupQueryFilter) (model.GroupList, error) {
	return s.store.Accounts().ListGroups(ctx, filter)
}

func (s *AccountsService) GetGroup(ctx context.Context, id uuid.UUID) (model.Group, error) {
	group, err := s.store.Accounts().GetGroup(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return model.Group{}, NewErrResourceNotFound(id, "group")
		}
		return model.Group{}, err
	}
	return group, nil
}

func (s *AccountsService) CreateGroup(ctx context.Context, group model.Group) (model.Group, error) {
	created, err := s.store.Accounts().CreateGroup(ctx, group)
	if err != nil {
		if errors.Is(err, store.ErrDuplicateKey) {
			return model.Group{}, NewErrDuplicateKey("group", fmt.Sprintf("%s/%s", group.Company, group.Name))
		}
		return model.Group{}, err
	}
	return created, nil
}

func (s *AccountsService) UpdateGroup(ctx context.Context, group model.Group) (model.Group, error) {
	result, err := s.store.Accounts().UpdateGroup(ctx, group)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return model.Group{}, NewErrResourceNotFound(group.ID, "group")
		}
		if errors.Is(err, store.ErrDuplicateKey) {
			return model.Group{}, NewErrDuplicateKey("group", fmt.Sprintf("%s/%s", group.Company, group.Name))
		}
		return model.Group{}, err
	}
	return result, nil
}

func (s *AccountsService) DeleteGroup(ctx context.Context, id uuid.UUID) error {
	return s.store.Accounts().DeleteGroup(ctx, id)
}

func (s *AccountsService) GetMember(ctx context.Context, username string) (model.Member, error) {
	member, err := s.store.Accounts().GetMember(ctx, username)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return model.Member{}, NewErrMemberNotFound(username)
		}
		return model.Member{}, err
	}
	return member, nil
}

// ListGroupMembers lists all members belonging to the specified group.
func (s *AccountsService) ListGroupMembers(ctx context.Context, groupID uuid.UUID) (model.MemberList, error) {
	if _, err := s.GetGroup(ctx, groupID); err != nil {
		return nil, err
	}
	return s.store.Accounts().ListMembers(ctx, store.NewMemberQueryFilter().ByGroupID(groupID))
}

// CreateMember creates a new member. Verifies the group exists.
func (s *AccountsService) CreateMember(ctx context.Context, member model.Member) (model.Member, error) {
	if _, err := s.GetGroup(ctx, member.GroupID); err != nil {
		return model.Member{}, err
	}
	created, err := s.store.Accounts().CreateMember(ctx, member)
	if err != nil {
		if errors.Is(err, store.ErrDuplicateKey) {
			return model.Member{}, NewErrDuplicateKey("member", member.Username)
		}
		return model.Member{}, err
	}
	return created, nil
}

// UpdateGroupMember updates a member within the specified group.
// The username is immutable; only other fields (e.g. email) are updated.
func (s *AccountsService) UpdateGroupMember(ctx context.Context, groupID uuid.UUID, username string, member model.Member) (model.Member, error) {
	if _, err := s.GetGroup(ctx, groupID); err != nil {
		return model.Member{}, err
	}

	existing, err := s.GetMember(ctx, username)
	if err != nil {
		return model.Member{}, err
	}

	if existing.GroupID != groupID {
		return model.Member{}, NewErrMembershipMismatch(username, groupID)
	}

	member.ID = existing.ID
	member.Username = existing.Username
	member.GroupID = existing.GroupID
	member.CreatedAt = existing.CreatedAt
	member.Group = nil

	result, err := s.store.Accounts().UpdateMember(ctx, member)
	if err != nil {
		if errors.Is(err, store.ErrRecordNotFound) {
			return model.Member{}, NewErrMemberNotFound(username)
		}
		return model.Member{}, err
	}
	return result, nil
}

// RemoveGroupMember removes a member from the specified group.
// Since a backend member must always belong to a group, this deletes the member.
func (s *AccountsService) RemoveGroupMember(ctx context.Context, groupID uuid.UUID, username string) error {
	if _, err := s.GetGroup(ctx, groupID); err != nil {
		return err
	}

	member, err := s.GetMember(ctx, username)
	if err != nil {
		return err
	}

	if member.GroupID != groupID {
		return NewErrMembershipMismatch(username, groupID)
	}

	return s.store.Accounts().DeleteMember(ctx, member.ID)
}
