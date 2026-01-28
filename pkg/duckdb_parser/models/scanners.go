package models

import (
	"database/sql/driver"
	"fmt"
)

// Disks is a slice of Disk that implements sql.Scanner for DuckDB LIST type.
type Disks []Disk

func (d *Disks) Scan(value interface{}) error {
	if value == nil {
		*d = nil
		return nil
	}
	slice, ok := value.([]interface{})
	if !ok {
		return fmt.Errorf("Disks.Scan: expected []interface{}, got %T", value)
	}
	result := make([]Disk, 0, len(slice))
	tracker := NewControllerTracker()

	for _, item := range slice {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		controller := toString(m["Controller"])
		controllerKey, bus := tracker.GetKeyAndBus(controller)

		disk := Disk{
			Key:           toString(m["Key"]),
			UnitNumber:    toInt32(m["UnitNumber"]),
			ControllerKey: controllerKey,
			File:          toString(m["File"]),
			Capacity:      toInt64(m["Capacity"]),
			Shared:        toBool(m["Shared"]),
			RDM:           toBool(m["RDM"]),
			Bus:           bus,
			Mode:          toString(m["Mode"]),
			Serial:        toString(m["Serial"]),
			Thin:          toString(m["Thin"]),
			Controller:    controller,
			Label:         toString(m["Label"]),
			SCSIUnit:      toString(m["SCSIUnit"]),
			Datastore:     Ref{ID: toString(m["Datastore"])},
		}
		result = append(result, disk)
	}
	*d = result
	return nil
}

func (d Disks) Value() (driver.Value, error) {
	return d, nil
}

// NICs is a slice of NIC that implements sql.Scanner for DuckDB LIST type.
type NICs []NIC

func (n *NICs) Scan(value interface{}) error {
	if value == nil {
		*n = nil
		return nil
	}
	slice, ok := value.([]interface{})
	if !ok {
		return fmt.Errorf("NICs.Scan: expected []interface{}, got %T", value)
	}
	result := make([]NIC, 0, len(slice))
	for _, item := range slice {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		nic := NIC{
			Network:         Ref{ID: toString(m["Network"])},
			MAC:             toString(m["MAC"]),
			Label:           toString(m["Label"]),
			Adapter:         toString(m["Adapter"]),
			Switch:          toString(m["Switch"]),
			Connected:       toBool(m["Connected"]),
			StartsConnected: toBool(m["StartsConnected"]),
			Type:            toString(m["Type"]),
			IPv4Address:     toString(m["IPv4Address"]),
			IPv6Address:     toString(m["IPv6Address"]),
		}
		result = append(result, nic)
	}
	*n = result
	return nil
}

func (n NICs) Value() (driver.Value, error) {
	return n, nil
}

// Networks is a slice of strings that implements sql.Scanner for DuckDB LIST type.
type Networks []string

func (n *Networks) Scan(value interface{}) error {
	if value == nil {
		*n = nil
		return nil
	}
	slice, ok := value.([]interface{})
	if !ok {
		return fmt.Errorf("Networks.Scan: expected []interface{}, got %T", value)
	}
	result := make([]string, 0, len(slice))
	for _, item := range slice {
		if s := toString(item); s != "" {
			result = append(result, s)
		}
	}
	*n = result
	return nil
}

func (n Networks) Value() (driver.Value, error) {
	return n, nil
}

// Concerns is a slice of Concern that implements sql.Scanner for DuckDB LIST type.
type Concerns []Concern

func (c *Concerns) Scan(value interface{}) error {
	if value == nil {
		*c = nil
		return nil
	}
	slice, ok := value.([]interface{})
	if !ok {
		return fmt.Errorf("Concerns.Scan: expected []interface{}, got %T", value)
	}
	result := make([]Concern, 0, len(slice))
	for _, item := range slice {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		concern := Concern{
			Id:         toString(m["Id"]),
			Label:      toString(m["Label"]),
			Category:   toString(m["Category"]),
			Assessment: toString(m["Assessment"]),
		}
		result = append(result, concern)
	}
	*c = result
	return nil
}

func (c Concerns) Value() (driver.Value, error) {
	return c, nil
}

// Helper functions for type conversion.

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func toInt64(v interface{}) int64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case int64:
		return val
	case int32:
		return int64(val)
	case int:
		return int64(val)
	case float64:
		return int64(val)
	case string:
		var i int64
		_, _ = fmt.Sscanf(val, "%d", &i)
		return i
	}
	return 0
}

func toInt32(v interface{}) int32 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case int32:
		return val
	case int64:
		return int32(val)
	case int:
		return int32(val)
	case float64:
		return int32(val)
	case string:
		var i int32
		_, _ = fmt.Sscanf(val, "%d", &i)
		return i
	}
	return 0
}

func toBool(v interface{}) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val == "true" || val == "True" || val == "1" || val == "Yes"
	case int64:
		return val != 0
	case int32:
		return val != 0
	case int:
		return val != 0
	}
	return false
}
