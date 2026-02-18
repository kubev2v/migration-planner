package calculators

import (
	"testing"
	"time"

	"github.com/kubev2v/migration-planner/internal/estimation"
)

func TestStorageMigration_Calculate_WithDefaultRate(t *testing.T) {
	t.Parallel()
	calc := NewStorageMigration()

	params := map[string]estimation.Param{
		ParamTotalDiskGB: {Key: ParamTotalDiskGB, Value: 1000.0},
	}

	result, err := calc.Calculate(params)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// 1000 GB / 500 = 2 units
	// 2 units * 110 minutes = 220 minutes
	expectedDuration := 220 * time.Minute
	if result.Duration != expectedDuration {
		t.Errorf("expected duration %v, got %v", expectedDuration, result.Duration)
	}
	if result.Reason == "" {
		t.Error("expected non-empty reason")
	}
}

func TestStorageMigration_Calculate_WithCustomRate(t *testing.T) {
	t.Parallel()
	calc := NewStorageMigration(WithMinutesPer500GB(200.0))

	params := map[string]estimation.Param{
		ParamTotalDiskGB: {Key: ParamTotalDiskGB, Value: 500.0},
	}

	result, err := calc.Calculate(params)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// 500 GB / 500 = 1 unit
	// 1 unit * 200 minutes = 200 minutes
	expectedDuration := 200 * time.Minute
	if result.Duration != expectedDuration {
		t.Errorf("expected duration %v, got %v", expectedDuration, result.Duration)
	}
}

func TestStorageMigration_Calculate_ZeroGB(t *testing.T) {
	t.Parallel()
	calc := NewStorageMigration()

	params := map[string]estimation.Param{
		ParamTotalDiskGB: {Key: ParamTotalDiskGB, Value: 0.0},
	}

	result, err := calc.Calculate(params)
	if err != nil {
		t.Fatalf("expected no error for zero GB, got: %v", err)
	}
	if result.Duration != 0 {
		t.Errorf("expected 0 duration for zero GB, got %v", result.Duration)
	}
}

func TestStorageMigration_Calculate_ErrorCases(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		params map[string]estimation.Param
	}{
		{
			name:   "missing total_disk_gb param",
			params: map[string]estimation.Param{},
		},
		{
			name: "invalid param type",
			params: map[string]estimation.Param{
				ParamTotalDiskGB: {Key: ParamTotalDiskGB, Value: "not a number"},
			},
		},
		{
			name: "negative disk size",
			params: map[string]estimation.Param{
				ParamTotalDiskGB: {Key: ParamTotalDiskGB, Value: -100.0},
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			calc := NewStorageMigration()
			_, err := calc.Calculate(tc.params)
			if err == nil {
				t.Errorf("expected error for case %q, got nil", tc.name)
			}
		})
	}
}
