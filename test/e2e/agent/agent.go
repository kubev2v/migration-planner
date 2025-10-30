package agent

// PlannerAgent defines the interface for interacting with a planner agent instance
type PlannerAgent interface {
	DumpLogs(string)
	GetIp() (string, error)
	IsServiceRunning(string, string) bool
	Run() error
	Restart() error
	Remove() error
}
