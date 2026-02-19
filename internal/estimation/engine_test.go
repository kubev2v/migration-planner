package estimation

import (
	"errors"
	"testing"
	"time"
)

// mockCalculator is a test double implementing the Calculator interface.
type mockCalculator struct {
	name   string
	result Estimation
	err    error
	// gotParams captures the params map passed to Calculate for inspection.
	gotParams map[string]Param
}

func (m *mockCalculator) Name() string { return m.name }
func (m *mockCalculator) Keys() []string {
	return nil
}
func (m *mockCalculator) Calculate(params map[string]Param) (Estimation, error) {
	m.gotParams = params
	return m.result, m.err
}

func TestNewEngine(t *testing.T) {
	t.Parallel()
	e := NewEngine()
	if e == nil {
		t.Fatal("expected non-nil Engine")
	}
	if len(e.calculators) != 0 {
		t.Errorf("expected 0 calculators, got %d", len(e.calculators))
	}
}

func TestRegister(t *testing.T) {
	t.Parallel()
	e := NewEngine()
	e.Register(&mockCalculator{name: "A"})
	e.Register(&mockCalculator{name: "B"})
	if len(e.calculators) != 2 {
		t.Errorf("expected 2 calculators, got %d", len(e.calculators))
	}
}

func TestRegister_PanicsOnDuplicate(t *testing.T) {
	t.Parallel()
	e := NewEngine()
	e.Register(&mockCalculator{name: "A"})
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate calculator name, got none")
		}
	}()
	e.Register(&mockCalculator{name: "A"})
}

func TestRun_ReturnsResultsKeyedByName(t *testing.T) {
	t.Parallel()
	e := NewEngine()
	e.Register(&mockCalculator{name: "calc-a", result: Estimation{Duration: 10 * time.Minute, Reason: "reason-a"}})
	e.Register(&mockCalculator{name: "calc-b", result: Estimation{Duration: 20 * time.Minute, Reason: "reason-b"}})

	results := e.Run(nil)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results["calc-a"].Duration != 10*time.Minute {
		t.Errorf("calc-a: expected 10m, got %v", results["calc-a"].Duration)
	}
	if results["calc-b"].Duration != 20*time.Minute {
		t.Errorf("calc-b: expected 20m, got %v", results["calc-b"].Duration)
	}
}

func TestRun_CalculatorErrorEncodedInReason(t *testing.T) {
	t.Parallel()
	e := NewEngine()
	e.Register(&mockCalculator{name: "failing", err: errors.New("something went wrong")})

	results := e.Run(nil)

	got, ok := results["failing"]
	if !ok {
		t.Fatal("expected result for failing calculator")
	}
	if got.Duration != 0 {
		t.Errorf("expected Duration 0 on error, got %v", got.Duration)
	}
	if got.Reason == "" {
		t.Error("expected non-empty Reason on error")
	}
}

func TestRun_InputSliceConvertedToMap(t *testing.T) {
	t.Parallel()
	calc := &mockCalculator{name: "spy"}
	e := NewEngine()
	e.Register(calc)

	inputs := []Param{
		{Key: "foo", Value: 1},
		{Key: "bar", Value: 2},
	}
	e.Run(inputs)

	if calc.gotParams["foo"].Value != 1 {
		t.Errorf("expected foo=1 in params map, got %v", calc.gotParams["foo"].Value)
	}
	if calc.gotParams["bar"].Value != 2 {
		t.Errorf("expected bar=2 in params map, got %v", calc.gotParams["bar"].Value)
	}
}

func TestRun_EmptyEngine(t *testing.T) {
	t.Parallel()
	e := NewEngine()
	results := e.Run([]Param{{Key: "x", Value: 42}})
	if len(results) != 0 {
		t.Errorf("expected empty results for engine with no calculators, got %d", len(results))
	}
}
