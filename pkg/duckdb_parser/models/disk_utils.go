package models

import "strings"

// controllerType holds the bus type and base key for a controller.
type controllerType struct {
	bus     string
	baseKey int32
}

// Controller type definitions with their base keys for CBT per-disk warnings.
var controllerTypes = map[string]controllerType{
	"ide":  {bus: "ide", baseKey: 200},
	"scsi": {bus: "scsi", baseKey: 1000},
	"sata": {bus: "sata", baseKey: 15000},
	"nvme": {bus: "nvme", baseKey: 20000},
}

// ControllerTracker generates unique controller keys per bus type.
type ControllerTracker struct {
	counts           map[string]int32
	controllerKeyMap map[string]int32
	controllerBusMap map[string]string
}

// NewControllerTracker creates a new ControllerTracker.
func NewControllerTracker() *ControllerTracker {
	return &ControllerTracker{
		counts:           make(map[string]int32),
		controllerKeyMap: make(map[string]int32),
		controllerBusMap: make(map[string]string),
	}
}

// GetKeyAndBus returns the controller key and bus type for a controller name.
// It caches results so the same controller name always returns the same key/bus.
func (ct *ControllerTracker) GetKeyAndBus(controllerName string) (key int32, bus string) {
	if key, ok := ct.controllerKeyMap[controllerName]; ok {
		return key, ct.controllerBusMap[controllerName]
	}

	ctype := parseControllerType(controllerName)
	key = ctype.baseKey + ct.counts[ctype.bus]
	ct.counts[ctype.bus]++

	ct.controllerKeyMap[controllerName] = key
	ct.controllerBusMap[controllerName] = ctype.bus

	return key, ctype.bus
}

// parseControllerType returns the controller type info from a controller name.
func parseControllerType(controllerName string) controllerType {
	nameLower := strings.ToLower(controllerName)
	for key, ct := range controllerTypes {
		if strings.Contains(nameLower, key) {
			return ct
		}
	}
	return controllerTypes["scsi"] // default
}
