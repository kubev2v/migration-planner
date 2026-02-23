package model

import (
	agentpkg "github.com/kubev2v/migration-planner/test/e2e/agent"
)

type E2EAgent struct {
	Agent agentpkg.PlannerAgent
	Api   *agentpkg.AgentApi
}
