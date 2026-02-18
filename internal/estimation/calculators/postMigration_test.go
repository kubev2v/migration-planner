package calculators

import (
	"testing"
	"time"

	"github.com/kubev2v/migration-planner/internal/estimation"
)

func TestPostMigrationTroubleShooting_NameAndKeys(t *testing.T) {
	t.Parallel()
	calc := NewPostMigrationTroubleShooting()

	if calc.Name() == "" {
		t.Error("expected non-empty Name()")
	}
	keys := calc.Keys()
	if len(keys) == 0 {
		t.Fatal("expected non-empty Keys()")
	}
	found := false
	for _, k := range keys {
		if k == ParamVMCount {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected Keys() to contain %q, got %v", ParamVMCount, keys)
	}
}

func TestPostMigrationTroubleShooting_Calculate_WithDefaults(t *testing.T) {
	t.Parallel()
	calc := NewPostMigrationTroubleShooting()

	params := map[string]estimation.Param{
		ParamVMCount: {Key: ParamVMCount, Value: 10},
	}

	result, err := calc.Calculate(params)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// 10 VMs * 60 mins (default) = 600 mins total
	// 600 mins / 10 engineers (default) = 60 mins
	expectedDuration := 60 * time.Minute
	if result.Duration != expectedDuration {
		t.Errorf("expected duration %v, got %v", expectedDuration, result.Duration)
	}
	if result.Reason == "" {
		t.Error("expected non-empty reason")
	}
}

func TestPostMigrationTroubleShooting_Calculate_WithCustomOptions(t *testing.T) {
	t.Parallel()
	calc := NewPostMigrationTroubleShooting(
		WithTroubleshootMinsPerVM(30.0),
		WithEngineerCount(3),
	)

	params := map[string]estimation.Param{
		ParamVMCount: {Key: ParamVMCount, Value: 12},
	}

	result, err := calc.Calculate(params)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// 12 VMs * 30 mins = 360 mins total
	// 360 mins / 3 engineers = 120 mins
	expectedDuration := 120 * time.Minute
	if result.Duration != expectedDuration {
		t.Errorf("expected duration %v, got %v", expectedDuration, result.Duration)
	}
	if result.Reason == "" {
		t.Error("expected non-empty reason")
	}
}

func TestPostMigrationTroubleShooting_Calculate_ErrorCases(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		calcOpts []PostMigrationTroubleshootingOption
		params   map[string]estimation.Param
	}{
		{
			name:   "missing vm_count param",
			params: map[string]estimation.Param{},
		},
		{
			name: "invalid vm_count type",
			params: map[string]estimation.Param{
				ParamVMCount: {Key: ParamVMCount, Value: "not a number"},
			},
		},
		{
			name: "negative vm_count",
			params: map[string]estimation.Param{
				ParamVMCount: {Key: ParamVMCount, Value: -5},
			},
		},
		{
			name:     "zero engineers via option",
			calcOpts: []PostMigrationTroubleshootingOption{WithEngineerCount(0)},
			params: map[string]estimation.Param{
				ParamVMCount: {Key: ParamVMCount, Value: 10},
			},
		},
		{
			name:     "negative engineers via option",
			calcOpts: []PostMigrationTroubleshootingOption{WithEngineerCount(-1)},
			params: map[string]estimation.Param{
				ParamVMCount: {Key: ParamVMCount, Value: 10},
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			calc := NewPostMigrationTroubleShooting(tc.calcOpts...)
			_, err := calc.Calculate(tc.params)
			if err == nil {
				t.Errorf("expected error for case %q, got nil", tc.name)
			}
		})
	}
}

func TestPostMigrationTroubleShooting_Calculate_ParamsOverrideDefaults(t *testing.T) {
	t.Parallel()
	calc := NewPostMigrationTroubleShooting()

	params := map[string]estimation.Param{
		ParamVMCount:                {Key: ParamVMCount, Value: 20},
		ParamTroubleshootMinsPerVM:  {Key: ParamTroubleshootMinsPerVM, Value: 45.0},
		ParamPostMigrationEngineers: {Key: ParamPostMigrationEngineers, Value: 5},
	}

	result, err := calc.Calculate(params)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// 20 VMs * 45 mins (from params) = 900 mins total
	// 900 mins / 5 engineers (from params) = 180 mins
	expectedDuration := 180 * time.Minute
	if result.Duration != expectedDuration {
		t.Errorf("expected duration %v, got %v", expectedDuration, result.Duration)
	}
	if result.Reason == "" {
		t.Error("expected non-empty reason")
	}
}

func TestPostMigrationTroubleShooting_Calculate_ParamsOverrideStructOptions(t *testing.T) {
	t.Parallel()
	calc := NewPostMigrationTroubleShooting(
		WithTroubleshootMinsPerVM(30.0),
		WithEngineerCount(3),
	)

	// params should take precedence over constructor options
	params := map[string]estimation.Param{
		ParamVMCount:                {Key: ParamVMCount, Value: 15},
		ParamTroubleshootMinsPerVM:  {Key: ParamTroubleshootMinsPerVM, Value: 50.0},
		ParamPostMigrationEngineers: {Key: ParamPostMigrationEngineers, Value: 6},
	}

	result, err := calc.Calculate(params)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// 15 VMs * 50 mins (from params, not struct) = 750 mins total
	// 750 mins / 6 engineers (from params, not struct) = 125 mins
	expectedDuration := 125 * time.Minute
	if result.Duration != expectedDuration {
		t.Errorf("expected duration %v, got %v", expectedDuration, result.Duration)
	}
}

func TestPostMigrationTroubleShooting_Calculate_PartialParamOverride(t *testing.T) {
	t.Parallel()
	calc := NewPostMigrationTroubleShooting()

	// only override minsPerVM; engineer count falls back to default (10)
	params := map[string]estimation.Param{
		ParamVMCount:               {Key: ParamVMCount, Value: 20},
		ParamTroubleshootMinsPerVM: {Key: ParamTroubleshootMinsPerVM, Value: 30.0},
	}

	result, err := calc.Calculate(params)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// 20 VMs * 30 mins (from params) = 600 mins total
	// 600 mins / 10 engineers (default) = 60 mins
	expectedDuration := 60 * time.Minute
	if result.Duration != expectedDuration {
		t.Errorf("expected duration %v, got %v", expectedDuration, result.Duration)
	}
}
