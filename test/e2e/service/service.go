package service

import (
	"fmt"

	"github.com/kubev2v/migration-planner/internal/auth"
	. "github.com/kubev2v/migration-planner/test/e2e/utils"
	"go.uber.org/zap"
)

const (
	apiV1SourcesPath            = "/api/v1/sources"
	apiV1AssessmentsPath        = "/api/v1/assessments"
	apiV1AssessmentsRVToolsPath = "/api/v1/assessments/rvtools"
	apiV1AssessmentsJobsPath    = "/api/v1/assessments/jobs"
)

// plannerService is the concrete implementation of PlannerService
type plannerService struct {
	api         *ServiceApi
	credentials *auth.User
}

// DefaultPlannerService initializes a planner service using default *auth.User credentials
func DefaultPlannerService() (*plannerService, error) {
	return NewPlannerService(DefaultUserAuth())
}

// NewPlannerService initializes the planner service with custom *auth.User credentials
func NewPlannerService(cred *auth.User) (*plannerService, error) {
	zap.S().Info("Initializing PlannerService...")
	serviceApi, err := NewServiceApi(cred)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize planner service API")
	}
	return &plannerService{api: serviceApi, credentials: cred}, nil
}
