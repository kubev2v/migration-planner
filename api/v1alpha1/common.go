package v1alpha1

func StringToAgentStatus(s string) AgentStatus {
	switch s {
	case string(AgentStatusError):
		return AgentStatusError
	case string(AgentStatusGatheringInitialInventory):
		return AgentStatusGatheringInitialInventory
	case string(AgentStatusUpToDate):
		return AgentStatusUpToDate
	case string(AgentStatusWaitingForCredentials):
		return AgentStatusWaitingForCredentials
	case string(AgentStatusNotConnected):
		return AgentStatusNotConnected
	default:
		return AgentStatusNotConnected
	}
}
