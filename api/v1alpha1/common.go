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
	default:
		return SourceStatusWaitingForCredentials
	}
}
