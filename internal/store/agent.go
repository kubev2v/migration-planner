package store

import (
	"context"
	"errors"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	apiAgent "github.com/kubev2v/migration-planner/api/v1alpha1/agent"
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
	List(ctx context.Context, filter *AgentQueryFilter, opts *AgentQueryOptions) (api.AgentList, error)
	Get(ctx context.Context, id string) (*api.Agent, error)
	Update(ctx context.Context, agentUpdate apiAgent.AgentStatusUpdate) (*api.Agent, error)
	UpdateSourceID(ctx context.Context, agentID string, sourceID string, associated bool) (*api.Agent, error)
	Create(ctx context.Context, agentUpdate apiAgent.AgentStatusUpdate) (*api.Agent, error)
	Delete(ctx context.Context, id string, softDeletion bool) error
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
func (a *AgentStore) List(ctx context.Context, filter *AgentQueryFilter, opts *AgentQueryOptions) (api.AgentList, error) {
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

	return agents.ToApiResource(), nil
}

// Create creates an agent from api model.
func (a *AgentStore) Create(ctx context.Context, agentUpdate apiAgent.AgentStatusUpdate) (*api.Agent, error) {
	agent := model.NewAgentFromApiResource(&agentUpdate)

	if err := a.getDB(ctx).WithContext(ctx).Create(agent).Error; err != nil {
		return nil, err
	}

	createdResource := agent.ToApiResource()
	return &createdResource, nil
}

// Update updates an agent from api model.
func (a *AgentStore) Update(ctx context.Context, agentUpdate apiAgent.AgentStatusUpdate) (*api.Agent, error) {
	agent := model.NewAgentFromApiResource(&agentUpdate)

	if err := a.getDB(ctx).WithContext(ctx).First(&model.Agent{ID: agentUpdate.Id}).Error; err != nil {
		return nil, err
	}

	if tx := a.getDB(ctx).WithContext(ctx).Clauses(clause.Returning{}).Updates(&agent); tx.Error != nil {
		return nil, tx.Error
	}

	updatedAgent := agent.ToApiResource()
	return &updatedAgent, nil
}

// UpdateSourceID updates the sources id field of an agent.
// The source must exists.
func (a *AgentStore) UpdateSourceID(ctx context.Context, agentID string, sourceID string, associated bool) (*api.Agent, error) {
	agent := model.NewAgentFromID(agentID)

	if err := a.getDB(ctx).WithContext(ctx).First(agent).Error; err != nil {
		return nil, err
	}

	agent.SourceID = &sourceID
	agent.Associated = associated

	if tx := a.getDB(ctx).WithContext(ctx).Clauses(clause.Returning{}).Updates(&agent); tx.Error != nil {
		return nil, tx.Error
	}

	updatedAgent := agent.ToApiResource()
	return &updatedAgent, nil
}

// Get returns an agent based on its id.
func (a *AgentStore) Get(ctx context.Context, id string) (*api.Agent, error) {
	agent := model.NewAgentFromID(id)

	if err := a.getDB(ctx).WithContext(ctx).Unscoped().First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRecordNotFound
		}
		return nil, err
	}

	agentSource := agent.ToApiResource()
	return &agentSource, nil
}

// Delete removes an agent.
// If softDeletion is true, the agent is soft-deleted.
func (a *AgentStore) Delete(ctx context.Context, id string, softDeletion bool) error {
	agent := model.NewAgentFromID(id)
	tx := a.getDB(ctx)
	if !softDeletion {
		tx = tx.Unscoped()
	}
	result := tx.Delete(&agent)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		zap.S().Named("agent_store").Infof("ERROR: %v", result.Error)
		return result.Error
	}
	return nil
}

func (a *AgentStore) getDB(ctx context.Context) *gorm.DB {
	tx := FromContext(ctx)
	if tx != nil {
		return tx
	}
	return a.db
}

type BaseAgentQuerier struct {
	QueryFn []func(tx *gorm.DB) *gorm.DB
}

type AgentQueryFilter = BaseAgentQuerier

func NewAgentQueryFilter() *AgentQueryFilter {
	return &AgentQueryFilter{QueryFn: make([]func(tx *gorm.DB) *gorm.DB, 0)}
}

func (qf *AgentQueryFilter) BySourceID(sourceID string) *AgentQueryFilter {
	qf.QueryFn = append(qf.QueryFn, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("source_id = ?", sourceID)
	})
	return qf
}

func (qf *AgentQueryFilter) BySoftDeleted(isSoftDeleted bool) *AgentQueryFilter {
	qf.QueryFn = append(qf.QueryFn, func(tx *gorm.DB) *gorm.DB {
		if isSoftDeleted {
			return tx.Unscoped().Where("deleted_at IS NOT NULL")
		}
		return tx
	})
	return qf
}

type AgentQueryOptions = BaseAgentQuerier

func NewAgentQueryOptions() *AgentQueryOptions {
	return &AgentQueryOptions{QueryFn: make([]func(tx *gorm.DB) *gorm.DB, 0)}
}

func (o *AgentQueryOptions) WithIncludeSoftDeleted(includeSoftDeleted bool) *AgentQueryOptions {
	o.QueryFn = append(o.QueryFn, func(tx *gorm.DB) *gorm.DB {
		if includeSoftDeleted {
			return tx.Unscoped()
		}
		return tx
	})
	return o
}

func (qf *AgentQueryFilter) ByID(ids []string) *AgentQueryFilter {
	qf.QueryFn = append(qf.QueryFn, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("id IN ?", ids)
	})
	return qf
}

func (o *AgentQueryOptions) WithSortOrder(sort SortOrder) *AgentQueryOptions {
	o.QueryFn = append(o.QueryFn, func(tx *gorm.DB) *gorm.DB {
		switch sort {
		case SortByID:
			return tx.Order("id")
		case SortByUpdatedTime:
			return tx.Order("updated_at")
		case SortByCreatedTime:
			return tx.Order("created_at")
		default:
			return tx
		}
	})
	return o
}
