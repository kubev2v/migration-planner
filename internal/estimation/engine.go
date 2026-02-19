package estimation

import "fmt"

// Engine orchestrates Calculator objects and aggregates their results
type Engine struct {
	calculators []Calculator
}

// NewEngine creates a new Engine with no calculators registered.
func NewEngine() *Engine {
	return &Engine{
		calculators: make([]Calculator, 0),
	}
}

// Register adds a Calculator to participate in the estimation.
// Calculators are executed in the order they are registered.
// Register panics if a calculator with the same Name() is already registered,
// as duplicate names would silently overwrite results in Run.
func (e *Engine) Register(c Calculator) {
	for _, existing := range e.calculators {
		if existing.Name() == c.Name() {
			panic(fmt.Sprintf("estimation: calculator %q already registered", c.Name()))
		}
	}
	e.calculators = append(e.calculators, c)
}

// Run executes all registered calculators against the provided params
func (e *Engine) Run(inputs []Param) map[string]Estimation {
	// Convert slice to map for lookups by Calculators
	paramMap := make(map[string]Param)
	for _, p := range inputs {
		paramMap[p.Key] = p
	}

	results := make(map[string]Estimation)
	// aggregate results
	// TODO: maybe separate errors to a different result object
	// TODO: in later phases, add different aggregations for parralelable calculations
	for _, calc := range e.calculators {
		est, err := calc.Calculate(paramMap)
		if err != nil {
			results[calc.Name()] = Estimation{
				Duration: 0,
				Reason:   fmt.Sprintf("Error: %v", err),
			}
			continue
		}
		results[calc.Name()] = est
	}
	return results
}
