package model

import (
	. "github.com/kubev2v/migration-planner/test/e2e/agent"
)

type E2EAgent struct {
	Agent PlannerAgent
	Api   *AgentApi
}
