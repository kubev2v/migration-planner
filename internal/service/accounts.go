package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"gopkg.in/yaml.v3"
)

type AccountsServicer interface {
	Initialize(ctx context.Context, adminGroup AdminGroup) error
	GetIdentity(ctx context.Context, authUser auth.User) (Identity, error)
	ListGroups(ctx context.Context, filter *store.GroupQueryFilter) (model.GroupList, error)
	GetGroup(ctx context.Context, id uuid.UUID) (model.Group, error)
	CreateGroup(ctx context.Context, group model.Group) (model.Group, error)
	UpdateGroup(ctx context.Context, group model.Group) (model.Group, error)
	DeleteGroup(ctx context.Context, id uuid.UUID) error
	GetMember(ctx context.Context, username string) (model.Member, error)
	ListGroupMembers(ctx context.Context, groupID uuid.UUID) (model.MemberList, error)
	CreateMember(ctx context.Context, member model.Member) (model.Member, error)
	UpdateGroupMember(ctx context.Context, groupID uuid.UUID, username string, member model.Member) (model.Member, error)
	RemoveGroupMember(ctx context.Context, groupID uuid.UUID, username string) error
}

type AdminGroupMember struct {
	Username string `yaml:"username"`
	Email    string `yaml:"email"`
}

type AdminGroup struct {
	Name    string             `yaml:"name"`
	Members []AdminGroupMember `yaml:"members"`
}

func ParseAdminGroupFile(path string) (*AdminGroup, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading admin group file: %w", err)
	}

	var ag AdminGroup
	if err := yaml.Unmarshal(data, &ag); err != nil {
		return nil, fmt.Errorf("parsing admin group file: %w", err)
	}

	ag.Name = strings.TrimSpace(ag.Name)
	if ag.Name == "" {
		return nil, fmt.Errorf("admin group file: name is required")
	}
	if len(ag.Members) == 0 {
		return nil, fmt.Errorf("admin group file: at least one member is required")
	}

	members := make(map[string]AdminGroupMember, len(ag.Members))
	for i, m := range ag.Members {
		m.Username = strings.TrimSpace(m.Username)
		m.Email = strings.TrimSpace(m.Email)
		if m.Username == "" {
			return nil, fmt.Errorf("admin group file: member[%d] username is required", i)
		}
		if m.Email == "" {
			return nil, fmt.Errorf("admin group file: member[%d] email is required", i)
		}
		if _, err := mail.ParseAddress(m.Email); err != nil {
			return nil, fmt.Errorf("admin group file: member[%d] invalid email %q: %w", i, m.Email, err)
		}
		if _, exists := members[m.Username]; exists {
			continue
		}
		members[m.Username] = m
	}

	ag.Members = make([]AdminGroupMember, 0, len(members))
	for _, m := range members {
		ag.Members = append(ag.Members, m)
	}

	return &ag, nil
}

type AccountsService struct {
	store store.Store
}

func NewAccountsService(store store.Store) *AccountsService {
	return &AccountsService{store: store}
}

func NewAccountsServicer(store store.Store) AccountsServicer {
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
	partners, err := s.store.PartnerCustomer().List(ctx,
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

func (s *AccountsService) Initialize(ctx context.Context, adminGroup AdminGroup) (retErr error) {
	ctx, err := s.store.NewTransactionContext(ctx)
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer func() {
		if retErr != nil {
			_, _ = store.Rollback(ctx)
		}
	}()

	groups, err := s.store.Accounts().ListGroups(ctx,
		store.NewGroupQueryFilter().ByName(adminGroup.Name).ByKind("admin"))
	if err != nil {
		return fmt.Errorf("looking up admin group: %w", err)
	}

	for _, g := range groups {
		if err := s.store.Accounts().DeleteGroup(ctx, g.ID); err != nil {
			return fmt.Errorf("deleting existing admin group %s: %w", g.ID, err)
		}
	}

	group, err := s.store.Accounts().CreateGroup(ctx, model.Group{
		ID:      uuid.New(),
		Name:    adminGroup.Name,
		Company: adminGroup.Name,
		Kind:    "admin",
	})
	if err != nil {
		return fmt.Errorf("creating admin group: %w", err)
	}

	for _, m := range adminGroup.Members {
		if _, err := s.store.Accounts().CreateMember(ctx, model.Member{
			Username: m.Username,
			Email:    m.Email,
			GroupID:  group.ID,
		}); err != nil {
			return fmt.Errorf("adding admin member %s: %w", m.Username, err)
		}
	}

	if _, err := store.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
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

	ctx, err := s.store.NewTransactionContext(ctx)
	if err != nil {
		return model.Member{}, err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	created, err := s.store.Accounts().CreateMember(ctx, member)
	if err != nil {
		if errors.Is(err, store.ErrDuplicateKey) {
			return model.Member{}, NewErrDuplicateKey("member", member.Username)
		}
		return model.Member{}, err
	}

	// Write org membership relation for authz
	updates := store.NewRelationshipBuilder().
		With(model.NewOrgResource(member.GroupID.String()), model.MemberRelation, model.NewUserSubject(member.Username)).
		Build()
	if err := s.store.Authz().WriteRelationships(ctx, updates); err != nil {
		return model.Member{}, fmt.Errorf("failed to write member authz relation: %w", err)
	}

	if _, err := store.Commit(ctx); err != nil {
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

	ctx, err = s.store.NewTransactionContext(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = store.Rollback(ctx)
	}()

	if err := s.store.Accounts().DeleteMember(ctx, member.ID); err != nil {
		return err
	}

	// Remove org membership relation from authz
	updates := store.NewRelationshipBuilder().
		Without(model.NewOrgResource(groupID.String()), model.MemberRelation, model.NewUserSubject(username)).
		Build()
	if err := s.store.Authz().WriteRelationships(ctx, updates); err != nil {
		return fmt.Errorf("failed to remove member authz relation: %w", err)
	}

	if _, err := store.Commit(ctx); err != nil {
		return err
	}

	return nil
}
