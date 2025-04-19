package e2e_helpers

import (
	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	. "github.com/kubev2v/migration-planner/test/e2e/e2e-service"
	. "github.com/kubev2v/migration-planner/test/e2e/e2e_agent"
	"go.uber.org/zap"
)

// CreateAgent Create VM with the UUID of the source created
func CreateAgent(idForTest string, uuid uuid.UUID, vmName string) (PlannerAgent, error) {
	zap.S().Info("Creating agent...")
	agent, err := NewPlannerAgent(uuid, vmName, idForTest)
	if err != nil {
		return nil, err
	}
	err = agent.Run()
	if err != nil {
		return nil, err
	}
	zap.S().Info("Agent created successfully")
	return agent, nil
}

// FindAgentIp store the ip case there is no error
func FindAgentIp(agent PlannerAgent, agentIP *string) error {
	zap.S().Info("Attempting to retrieve agent IP")
	ip, err := agent.GetIp()
	if err != nil {
		return err
	}
	*agentIP = ip
	return nil
}

func IsPlannerAgentRunning(agent PlannerAgent, agentIP string) bool {
	return agent.IsServiceRunning(agentIP, "planner-agent")
}

// AgentIsUpToDate helper function to check that source is up to date eventually
func AgentIsUpToDate(svc PlannerService, uuid uuid.UUID) bool {
	source, err := svc.GetSource(uuid)
	if err != nil {
		zap.S().Errorf("Error getting source.")
		return false
	}
	zap.S().Infof("agent status is: %s", string(source.Agent.Status))
	return source.Agent.Status == v1alpha1.AgentStatusUpToDate
}

// CredentialURL helper function for wait until the service return correct credential url for a source by UUID
func CredentialURL(svc PlannerService, uuid uuid.UUID) string {
	zap.S().Info("try to retrieve valid credentials url")
	s, err := svc.GetSource(uuid)
	if err != nil {
		return ""
	}
	if s.Agent == nil {
		return ""
	}
	if s.Agent.CredentialUrl != "N/A" && s.Agent.CredentialUrl != "" {
		return s.Agent.CredentialUrl
	}

	return ""
}
