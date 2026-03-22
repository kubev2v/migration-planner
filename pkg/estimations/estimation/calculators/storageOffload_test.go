package calculators

import (
	"testing"
	"time"

	"github.com/kubev2v/migration-planner/pkg/estimations/estimation"
)

func TestStorageOffload_Name(t *testing.T) {
	t.Parallel()
	calc := NewStorageOffload()
	if calc.Name() != "Storage Offload" {
		t.Errorf("unexpected name: %s", calc.Name())
	}
}

func TestStorageOffload_Keys(t *testing.T) {
	t.Parallel()
	calc := NewStorageOffload()
	keys := calc.Keys()
	if len(keys) != 1 || keys[0] != ParamTotalDiskGB {
		t.Errorf("unexpected keys: %v", keys)
	}
}

func TestStorageOffload_Calculate_1000GB(t *testing.T) {
	t.Parallel()
	calc := NewStorageOffload()
	params := map[string]estimation.Param{
		ParamTotalDiskGB: {Key: ParamTotalDiskGB, Value: 1000.0},
	}
	result, err := calc.Calculate(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsRanged() {
		t.Fatal("expected ranged estimation")
	}
	// At 2 GB/s: 1000 / 2 = 500 s
	expectedMin := time.Duration(500 * float64(time.Second))
	// At 0.5 GB/s: 1000 / 0.5 = 2000 s
	expectedMax := time.Duration(2000 * float64(time.Second))
	if *result.MinDuration != expectedMin {
		t.Errorf("min: expected %v, got %v", expectedMin, *result.MinDuration)
	}
	if *result.MaxDuration != expectedMax {
		t.Errorf("max: expected %v, got %v", expectedMax, *result.MaxDuration)
	}
	if result.Reason == "" {
		t.Error("expected non-empty reason")
	}
}

func TestStorageOffload_Calculate_ZeroGB(t *testing.T) {
	t.Parallel()
	calc := NewStorageOffload()
	params := map[string]estimation.Param{
		ParamTotalDiskGB: {Key: ParamTotalDiskGB, Value: 0.0},
	}
	result, err := calc.Calculate(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *result.MinDuration != 0 || *result.MaxDuration != 0 {
		t.Error("expected zero durations for zero GB")
	}
}

func TestStorageOffload_Calculate_ErrorCases(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		params map[string]estimation.Param
	}{
		{"missing param", map[string]estimation.Param{}},
		{"negative disk", map[string]estimation.Param{
			ParamTotalDiskGB: {Key: ParamTotalDiskGB, Value: -1.0},
		}},
		{"invalid type", map[string]estimation.Param{
			ParamTotalDiskGB: {Key: ParamTotalDiskGB, Value: "bad"},
		}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewStorageOffload().Calculate(tc.params)
			if err == nil {
				t.Errorf("expected error for %q, got nil", tc.name)
			}
		})
	}
}
