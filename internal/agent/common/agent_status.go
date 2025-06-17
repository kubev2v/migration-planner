package common

type AgentStatus struct {
	Connected                  bool
	StateUpdateSuccessfull     *bool // nil means the request has not been made yet
	InventoryUpdateSuccessfull *bool // nil means the request has not been made yet
	InventoryUpdateError       error
	StateUpdateError           error
}
