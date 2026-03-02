package estimation

import (
	"time"
)

// Calculator encapsulates one specific part of the estimation (e.g. "post migration troubleshooting", "storage migration").
type Calculator interface {
	// Name returns the human-readable name of this calculator, used as the key in Engine results.
	Name() string
	// Keys returns the list of Param keys this calculator depends on.
	Keys() []string
	// Calculate runs the estimation using the provided params and returns an Estimation or an error.
	Calculate(params map[string]Param) (Estimation, error)
}

// Param represents an input for a Calculator (can be either user supplied or discovered)
type Param struct {
	Key   string      // Unique identifier (e.g., "network_bandwidth")
	Value interface{} // The actual value (e.g., 1000, "fast", 0.8)
}

// Estimation the result of a Calculator calculation
type Estimation struct {
	Duration time.Duration
	Reason   string
}
