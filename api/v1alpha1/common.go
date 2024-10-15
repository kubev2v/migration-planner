package v1alpha1

func StringToSourceStatus(s string) SourceStatus {
	switch s {
	case string(SourceStatusError):
		return SourceStatusError
	case string(SourceStatusGatheringInitialInventory):
		return SourceStatusGatheringInitialInventory
	case string(SourceStatusUpToDate):
		return SourceStatusUpToDate
	case string(SourceStatusWaitingForCredentials):
		return SourceStatusWaitingForCredentials
	case string(SourceStatusNotConnected):
		return SourceStatusNotConnected
	default:
		return SourceStatusNotConnected
	}
}

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
