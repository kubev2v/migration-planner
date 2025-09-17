package service

import (
	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
)

// PlannerService defines the interface for interacting with the planner service
type PlannerService interface {
	sourceApi
	imageApi
}

type sourceApi interface {
	CreateSource(string) (*v1alpha1.Source, error)
	GetSource(uuid.UUID) (*v1alpha1.Source, error)
	GetSources() (*v1alpha1.SourceList, error)
	RemoveSource(uuid.UUID) error
	RemoveSources() error
	UpdateSource(uuid.UUID, *v1alpha1.Inventory) error
}

type imageApi interface {
	GetImageUrl(uuid.UUID) (string, error)
}
