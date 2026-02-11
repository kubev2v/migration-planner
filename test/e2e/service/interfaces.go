package service

import (
	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
)

// PlannerService defines the interface for interacting with the planner service
type PlannerService interface {
	sourceApi
	imageApi
	assessmentApi
	jobApi
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

type assessmentApi interface {
	CreateAssessment(name, sourceType string, sourceId *uuid.UUID, inventory *v1alpha1.Inventory) (*v1alpha1.Assessment, error)
	CreateAssessmentFromRvtools(name, filepath string) (*v1alpha1.Assessment, error)
	GetAssessment(uuid.UUID) (*v1alpha1.Assessment, error)
	GetAssessments() (*v1alpha1.AssessmentListResponse, error)
	UpdateAssessment(uuid.UUID, string) (*v1alpha1.Assessment, error)
	RemoveAssessment(uuid.UUID) error
}

type jobApi interface {
	CreateRVToolsJob(name, filepath string) (*v1alpha1.Job, error)
	GetJob(id int64) (*v1alpha1.Job, error)
	CancelJob(id int64) (*v1alpha1.Job, error)
}
