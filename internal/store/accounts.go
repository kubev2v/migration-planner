package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kubev2v/migration-planner/internal/store/model"
)

type Accounts interface {
	// Groups
	ListGroups(ctx context.Context, filter *GroupQueryFilter) (model.GroupList, error)
	GetGroup(ctx context.Context, id uuid.UUID) (model.Group, error)
	CreateGroup(ctx context.Context, group model.Group) (model.Group, error)
	UpdateGroup(ctx context.Context, group model.Group) (model.Group, error)
	DeleteGroup(ctx context.Context, id uuid.UUID) error

	// Members
	ListMembers(ctx context.Context, filter *MemberQueryFilter) (model.MemberList, error)
	GetMember(ctx context.Context, username string) (model.Member, error)
	CreateMember(ctx context.Context, member model.Member) (model.Member, error)
	UpdateMember(ctx context.Context, member model.Member) (model.Member, error)
	DeleteMember(ctx context.Context, id uuid.UUID) error
}

type AccountsStore struct {
	db *gorm.DB
}

var _ Accounts = (*AccountsStore)(nil)

func NewAccountsStore(db *gorm.DB) Accounts {
	return &AccountsStore{db: db}
}

func (s *AccountsStore) ListGroups(ctx context.Context, filter *GroupQueryFilter) (model.GroupList, error) {
	var groups model.GroupList
	tx := s.getDB(ctx).Model(&groups).Order("created_at DESC").Preload("Members")

	if filter != nil {
		for _, fn := range filter.QueryFn {
			tx = fn(tx)
		}
	}

	result := tx.Find(&groups)
	if result.Error != nil {
		return nil, result.Error
	}
	return groups, nil
}

func (s *AccountsStore) GetGroup(ctx context.Context, id uuid.UUID) (model.Group, error) {
	var group model.Group
	result := s.getDB(ctx).Preload("Members").First(&group, "id = ?", id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return model.Group{}, ErrRecordNotFound
		}
		return model.Group{}, result.Error
	}
	return group, nil
}

func (s *AccountsStore) CreateGroup(ctx context.Context, group model.Group) (model.Group, error) {
	result := s.getDB(ctx).Clauses(clause.Returning{}).Create(&group)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return model.Group{}, ErrDuplicateKey
		}
		return model.Group{}, result.Error
	}
	return group, nil
}

func (s *AccountsStore) UpdateGroup(ctx context.Context, group model.Group) (model.Group, error) {
	if err := s.getDB(ctx).First(&model.Group{}, "id = ?", group.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.Group{}, ErrRecordNotFound
		}
		return model.Group{}, err
	}

	now := time.Now()
	group.UpdatedAt = &now
	if err := s.getDB(ctx).Model(&group).Updates(&group).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return model.Group{}, ErrDuplicateKey
		}
		return model.Group{}, err
	}
	return group, nil
}

func (s *AccountsStore) DeleteGroup(ctx context.Context, id uuid.UUID) error {
	result := s.getDB(ctx).Unscoped().Delete(&model.Group{}, "id = ?", id.String())
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return result.Error
	}
	return nil
}

func (s *AccountsStore) ListMembers(ctx context.Context, filter *MemberQueryFilter) (model.MemberList, error) {
	var members model.MemberList
	tx := s.getDB(ctx).Model(&members).Order("created_at DESC").Preload("Group")

	if filter != nil {
		for _, fn := range filter.QueryFn {
			tx = fn(tx)
		}
	}

	result := tx.Find(&members)
	if result.Error != nil {
		return nil, result.Error
	}
	return members, nil
}

func (s *AccountsStore) GetMember(ctx context.Context, username string) (model.Member, error) {
	var member model.Member
	result := s.getDB(ctx).Preload("Group").First(&member, "username = ?", username)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return model.Member{}, ErrRecordNotFound
		}
		return model.Member{}, result.Error
	}
	return member, nil
}

func (s *AccountsStore) CreateMember(ctx context.Context, member model.Member) (model.Member, error) {
	if member.ID == uuid.Nil {
		member.ID = uuid.New()
	}
	result := s.getDB(ctx).Clauses(clause.Returning{}).Create(&member)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return model.Member{}, ErrDuplicateKey
		}
		return model.Member{}, result.Error
	}
	return member, nil
}

func (s *AccountsStore) UpdateMember(ctx context.Context, member model.Member) (model.Member, error) {
	if err := s.getDB(ctx).First(&model.Member{}, "id = ?", member.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.Member{}, ErrRecordNotFound
		}
		return model.Member{}, err
	}

	now := time.Now()
	member.UpdatedAt = &now
	if err := s.getDB(ctx).Model(&member).Updates(&member).Error; err != nil {
		return model.Member{}, err
	}
	return member, nil
}

func (s *AccountsStore) DeleteMember(ctx context.Context, id uuid.UUID) error {
	result := s.getDB(ctx).Unscoped().Delete(&model.Member{}, "id = ?", id)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return result.Error
	}
	return nil
}

func (s *AccountsStore) getDB(ctx context.Context) *gorm.DB {
	tx := FromContext(ctx)
	if tx != nil {
		return tx
	}
	return s.db
}
