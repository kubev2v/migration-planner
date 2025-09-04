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

type AgentQueryOptions BaseQuerier

func NewAgentQueryOptions() *AgentQueryOptions {
	return &AgentQueryOptions{QueryFn: make([]func(tx *gorm.DB) *gorm.DB, 0)}
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

func (sf *SourceQueryFilter) WithoutDefaultInventory() *SourceQueryFilter {
	sf.QueryFn = append(sf.QueryFn, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("id != ?", uuid.UUID{})
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

type AssessmentQueryFilter struct {
	QueryFn []func(*gorm.DB) *gorm.DB
}

type AssessmentQueryOptions struct {
	QueryFn []func(*gorm.DB) *gorm.DB
}

func NewAssessmentQueryFilter() *AssessmentQueryFilter {
	return &AssessmentQueryFilter{}
}

func NewAssessmentQueryOptions() *AssessmentQueryOptions {
	return &AssessmentQueryOptions{}
}

// Filter by organization ID
func (f *AssessmentQueryFilter) WithOrgID(orgID string) *AssessmentQueryFilter {
	f.QueryFn = append(f.QueryFn, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("org_id = ?", orgID)
	})
	return f
}

// Filter by source
func (f *AssessmentQueryFilter) WithSourceType(sourceType string) *AssessmentQueryFilter {
	f.QueryFn = append(f.QueryFn, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("source_type = ?", sourceType)
	})
	return f
}

// Filter by source ID
func (f *AssessmentQueryFilter) WithSourceID(sourceID string) *AssessmentQueryFilter {
	f.QueryFn = append(f.QueryFn, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("source_id = ?", sourceID)
	})
	return f
}

// Filter by name pattern
func (f *AssessmentQueryFilter) WithNameLike(pattern string) *AssessmentQueryFilter {
	f.QueryFn = append(f.QueryFn, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("name ILIKE ?", "%"+pattern+"%")
	})
	return f
}

// Limit results
func (o *AssessmentQueryOptions) WithLimit(limit int) *AssessmentQueryOptions {
	o.QueryFn = append(o.QueryFn, func(tx *gorm.DB) *gorm.DB {
		return tx.Limit(limit)
	})
	return o
}

// Offset results
func (o *AssessmentQueryOptions) WithOffset(offset int) *AssessmentQueryOptions {
	o.QueryFn = append(o.QueryFn, func(tx *gorm.DB) *gorm.DB {
		return tx.Offset(offset)
	})
	return o
}

// Order by specific field
func (o *AssessmentQueryOptions) WithOrder(order string) *AssessmentQueryOptions {
	o.QueryFn = append(o.QueryFn, func(tx *gorm.DB) *gorm.DB {
		return tx.Order(order)
	})
	return o
}
