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
	ID         openapi_types.UUID `json:"id" gorm:"primaryKey"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt `gorm:"index"`
	Name       string
	Status     string
	StatusInfo string
	Inventory  *JSONField[api.Inventory] `gorm:"type:jsonb"`
	CredUrl    string
}

type SourceList []Source

func (s Source) String() string {
	val, _ := json.Marshal(s)
	return string(val)
}

func NewSourceFromApiCreateResource(resource *api.SourceCreate) *Source {
	return &Source{ID: uuid.New(), Name: resource.Name}
}

func NewSourceFromId(id uuid.UUID) *Source {
	s := Source{ID: id}
	return &s
}

func (s *Source) ToApiResource() api.Source {
	inventory := api.Inventory{}
	if s.Inventory != nil {
		inventory = s.Inventory.Data
	}
	return api.Source{
		Id:            s.ID,
		Name:          s.Name,
		Status:        api.StringToSourceStatus(s.Status),
		StatusInfo:    s.StatusInfo,
		Inventory:     inventory,
		CredentialUrl: s.CredUrl,
		CreatedAt:     s.CreatedAt,
		UpdatedAt:     s.UpdatedAt,
	}
}

func (sl SourceList) ToApiResource() api.SourceList {
	sourceList := make([]api.Source, len(sl))
	for i, source := range sl {
		sourceList[i] = source.ToApiResource()
	}
	return sourceList
}
