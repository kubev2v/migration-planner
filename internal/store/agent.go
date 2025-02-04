package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/store/model"
	"go.uber.org/zap"
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
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteAll(ctx context.Context) error
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
//
// If includeSoftDeleted is true, it lists the agents soft-deleted.
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
	agent := model.NewAgentFromID(id)

	if err := a.getDB(ctx).WithContext(ctx).Unscoped().First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRecordNotFound
		}
		return nil, err
	}

	return agent, nil
}

// Delete removes an agent.
// If softDeletion is true, the agent is soft-deleted.
func (a *AgentStore) Delete(ctx context.Context, id uuid.UUID) error {
	agent := model.NewAgentFromID(id)

	result := a.getDB(ctx).Unscoped().Delete(&agent)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		zap.S().Named("agent_store").Infof("ERROR: %v", result.Error)
		return result.Error
	}
	return nil
}

func (a *AgentStore) DeleteAll(ctx context.Context) error {
	result := a.getDB(ctx).Unscoped().Exec("DELETE FROM agents")
	return result.Error
}

func (a *AgentStore) getDB(ctx context.Context) *gorm.DB {
	tx := FromContext(ctx)
	if tx != nil {
		return tx
	}
	return a.db
}
