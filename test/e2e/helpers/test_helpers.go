package helpers

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	. "github.com/kubev2v/migration-planner/test/e2e/agent"
	. "github.com/kubev2v/migration-planner/test/e2e/service"
	"go.uber.org/zap"
)

// CreateAgent Create VM with the UUID of the source created
func CreateAgent(sourceId uuid.UUID, vmName string, svc PlannerService) (PlannerAgent, error) {
	zap.S().Info("Creating agent...")
	url, err := svc.GetImageUrl(sourceId)
	if err != nil {
		return nil, fmt.Errorf("failed to get image url: %w", err)
	}

	agent, err := NewPlannerAgent(vmName, url)
	if err != nil {
		return nil, err
	}
	err = agent.Run()
	if err != nil {
		_ = agent.Remove()
		return nil, err
	}
	zap.S().Info("agent created successfully")
	return agent, nil
}

// AgentIsUpToDate helper function to check that source is up to date eventually
func AgentIsUpToDate(svc PlannerService, uuid uuid.UUID) (bool, error) {
	source, err := svc.GetSource(uuid)
	if err != nil {
		return false, fmt.Errorf("error getting source")
	}
	zap.S().Infof("agent status is: %s", string(source.Agent.Status))
	return source.Agent.Status == v1alpha1.AgentStatusUpToDate, nil
}

// CredentialURL helper function which return credential url for an agent by source UUID
func CredentialURL(svc PlannerService, uuid uuid.UUID) (string, error) {
	zap.S().Info("try to retrieve valid credentials url")
	s, err := svc.GetSource(uuid)
	if err != nil {
		return "", fmt.Errorf("error getting source")
	}
	if s.Agent == nil {
		return "", fmt.Errorf("source has no agent")
	}
	return s.Agent.CredentialUrl, nil
}

func GenerateVmName() string {
	VMNamePrefix := "e2e-agent-vm"
	return fmt.Sprintf("%s-%s", VMNamePrefix, uuid.New())
}
