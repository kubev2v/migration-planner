package model

import (
	"encoding/json"
	"strconv"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"gorm.io/gorm"
)

type Source struct {
	gorm.Model
	Name       string
	Status     string
	StatusInfo string
	Inventory  string
}

type SourceList []Source

func (s Source) String() string {
	val, _ := json.Marshal(s)
	return string(val)
}

func NewSourceFromApiCreateResource(resource *api.SourceCreate) *Source {
	return &Source{Name: resource.Name}
}

func NewSourceFromId(id uint) *Source {
	s := Source{}
	s.ID = id
	return &s
}

func (s *Source) ToApiResource() api.Source {
	return api.Source{
		Id:         strconv.FormatUint(uint64(s.ID), 10),
		Name:       s.Name,
		Status:     api.StringToSourceStatus(s.Status),
		StatusInfo: s.StatusInfo,
		Inventory:  s.Inventory,
		CreatedAt:  s.CreatedAt,
		UpdatedAt:  s.UpdatedAt,
	}
}

func (sl SourceList) ToApiResource() api.SourceList {
	sourceList := make([]api.Source, len(sl))
	for i, source := range sl {
		sourceList[i] = source.ToApiResource()
	}
	return sourceList
}
