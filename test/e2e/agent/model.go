package agent

type AgentModeRequest struct {
	Mode string `json:"mode"`
}

type AgentStatus struct {
	Mode              string `json:"mode"`
	ConsoleConnection string `json:"console_connection"`
	Error             string `json:"error,omitempty"`
}

type CollectorStartRequest struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type CollectorStatus struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type AgentApiCollectorStatus string
type AgentMode string

var (
	CollectorStatusCollected AgentApiCollectorStatus = "collected"
	CollectorStatusError     AgentApiCollectorStatus = "error"
)

var (
	AgentModeConnected AgentMode = "connected"
)
