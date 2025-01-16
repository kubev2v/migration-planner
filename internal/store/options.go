package store

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type BaseQuerier struct {
	QueryFn []func(tx *gorm.DB) *gorm.DB
}

type AgentQueryFilter BaseQuerier

func NewAgentQueryFilter() *AgentQueryFilter {
	return &AgentQueryFilter{QueryFn: make([]func(tx *gorm.DB) *gorm.DB, 0)}
}

func (qf *AgentQueryFilter) BySourceID(sourceID string) *AgentQueryFilter {
	qf.QueryFn = append(qf.QueryFn, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("source_id = ?", sourceID)
	})
	return qf
}

func (qf *AgentQueryFilter) ByUsername(username string) *AgentQueryFilter {
	qf.QueryFn = append(qf.QueryFn, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("username = ?", username)
	})
	return qf
}

func (qf *AgentQueryFilter) ByOrgID(id string) *AgentQueryFilter {
	qf.QueryFn = append(qf.QueryFn, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("org_id = ?", id)
	})
	return qf
}

func (qf *AgentQueryFilter) ByDefaultInventory() *AgentQueryFilter {
	qf.QueryFn = append(qf.QueryFn, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("id = ?", uuid.UUID{})
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

type AgentQueryOptions BaseQuerier

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

type SourceQueryFilter BaseQuerier

func NewSourceQueryFilter() *SourceQueryFilter {
	return &SourceQueryFilter{QueryFn: make([]func(tx *gorm.DB) *gorm.DB, 0)}
}

func (sf *SourceQueryFilter) ByUsername(username string) *SourceQueryFilter {
	sf.QueryFn = append(sf.QueryFn, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("username = ?", username)
	})
	return sf
}

func (sf *SourceQueryFilter) ByOrgID(id string) *SourceQueryFilter {
	sf.QueryFn = append(sf.QueryFn, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("org_id = ?", id)
	})
	return sf
}

func (sf *SourceQueryFilter) ByDefaultInventory() *SourceQueryFilter {
	sf.QueryFn = append(sf.QueryFn, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("id = ?", uuid.UUID{})
	})
	return sf
}

func (qf *SourceQueryFilter) ByOnPremises(isOnPremises bool) *SourceQueryFilter {
	qf.QueryFn = append(qf.QueryFn, func(tx *gorm.DB) *gorm.DB {
		if isOnPremises {
			return tx.Where("on_premises IS TRUE")
		}
		return tx.Where("on_premises IS NOT TRUE")
	})
	return qf
}
