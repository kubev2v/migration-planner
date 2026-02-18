package calculators

import (
	"fmt"

	"github.com/kubev2v/migration-planner/internal/estimation"
)

func getInt(p estimation.Param) (int, error) {
	switch v := p.Value.(type) {
	case float64:
		return int(v), nil // JSON default
	case int:
		return v, nil // Direct struct usage or YAML (sometimes)
	case int64:
		return int(v), nil
	default:
		return 0, fmt.Errorf("param %s is not a number (type: %T)", p.Key, p.Value)
	}
}

func getFloat(p estimation.Param) (float64, error) {
	switch v := p.Value.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	default:
		return 0.0, fmt.Errorf("param %s is not a number (type: %T)", p.Key, p.Value)
	}
}
