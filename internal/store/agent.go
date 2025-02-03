package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SortOrder int

const (
	Unsorted SortOrder = iota
	SortByID
	SortByUpdatedTime
	SortByCreatedTime
)

type Agent interface {
	List(ctx context.Context, filter *AgentQueryFilter, opts *AgentQueryOptions) (model.AgentList, error)
	Get(ctx context.Context, id uuid.UUID) (*model.Agent, error)
	Update(ctx context.Context, agent model.Agent) (*model.Agent, error)
	Create(ctx context.Context, agent model.Agent) (*model.Agent, error)
	InitialMigration(context.Context) error
}

type AgentStore struct {
	db *gorm.DB
}

func NewAgentSource(db *gorm.DB) Agent {
	return &AgentStore{db: db}
}

func (a *AgentStore) InitialMigration(ctx context.Context) error {
	return a.getDB(ctx).AutoMigrate(&model.Agent{})
}

// List lists all the agents.
func (a *AgentStore) List(ctx context.Context, filter *AgentQueryFilter, opts *AgentQueryOptions) (model.AgentList, error) {
	var agents model.AgentList
	tx := a.getDB(ctx)

	if filter != nil {
		for _, fn := range filter.QueryFn {
			tx = fn(tx)
		}
	}

	if opts != nil {
		for _, fn := range opts.QueryFn {
			tx = fn(tx)
		}
	}

	if err := tx.Model(&agents).Find(&agents).Error; err != nil {
		return nil, err
	}

	return agents, nil
}

// Create creates an agent.
func (a *AgentStore) Create(ctx context.Context, agent model.Agent) (*model.Agent, error) {
	if err := a.getDB(ctx).WithContext(ctx).Create(&agent).Error; err != nil {
		return nil, err
	}

	return &agent, nil
}

// Update updates an agent.
func (a *AgentStore) Update(ctx context.Context, agent model.Agent) (*model.Agent, error) {
	if err := a.getDB(ctx).WithContext(ctx).First(&model.Agent{ID: agent.ID}).Error; err != nil {
		return nil, err
	}

	if tx := a.getDB(ctx).WithContext(ctx).Clauses(clause.Returning{}).Updates(&agent); tx.Error != nil {
		return nil, tx.Error
	}

	return &agent, nil
}

// Get returns an agent based on its id.
func (a *AgentStore) Get(ctx context.Context, id uuid.UUID) (*model.Agent, error) {
	agent := &model.Agent{ID: id}

	if err := a.getDB(ctx).WithContext(ctx).Unscoped().First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRecordNotFound
		}
		return nil, err
	}

	return agent, nil
}

func (a *AgentStore) getDB(ctx context.Context) *gorm.DB {
	tx := FromContext(ctx)
	if tx != nil {
		return tx
	}
	return a.db
}
