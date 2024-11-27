package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"gorm.io/gorm"
)

type Source struct {
	ID        openapi_types.UUID `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt            `gorm:"index"`
	Inventory *JSONField[api.Inventory] `gorm:"type:jsonb"`
	Agents    []Agent                   `gorm:"constraint:OnDelete:SET NULL;"`
}

type SourceList []Source

func (s Source) String() string {
	val, _ := json.Marshal(s)
	return string(val)
}

func NewSourceFromApiCreateResource(id uuid.UUID) *Source {
	return &Source{ID: id}
}

func NewSourceFromId(id uuid.UUID) *Source {
	s := Source{ID: id}
	return &s
}

func (s *Source) ToApiResource() api.Source {
	source := api.Source{
		Id:        s.ID,
		Inventory: nil,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}

	if len(s.Agents) > 0 {
		agents := make([]api.SourceAgentItem, 0, len(s.Agents))
		for _, a := range s.Agents {
			agents = append(agents, api.SourceAgentItem{Id: uuid.MustParse(a.ID), Associated: a.Associated})
		}
		source.Agents = &agents
	}

	if s.Inventory != nil {
		source.Inventory = &s.Inventory.Data
	}
	return source
}

func (sl SourceList) ToApiResource() api.SourceList {
	sourceList := make([]api.Source, len(sl))
	for i, source := range sl {
		sourceList[i] = source.ToApiResource()
	}
	return sourceList
}
