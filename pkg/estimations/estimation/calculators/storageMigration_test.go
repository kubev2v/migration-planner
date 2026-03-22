package calculators

import (
	"testing"
	"time"

	"github.com/kubev2v/migration-planner/pkg/estimations/estimation"
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

	// (1000 GB * 1024 MB/GB) / (620 Mbps / 8) / 60 s/min
	expectedMins := (1000.0 * 1024.0) / (DefaultTransferRateMbps / 8) / 60.0
	expectedDuration := time.Duration(expectedMins * float64(time.Minute))
	if result.Duration == nil {
		t.Fatal("expected point estimation, got ranged")
	}
	if *result.Duration != expectedDuration {
		t.Errorf("expected duration %v, got %v", expectedDuration, *result.Duration)
	}
	if result.Reason == "" {
		t.Error("expected non-empty reason")
	}
}

func TestStorageMigration_Calculate_WithCustomRate(t *testing.T) {
	t.Parallel()
	const customRate = 1600.0 // Mbps
	calc := NewStorageMigration(WithTransferRateMbps(customRate))

	params := map[string]estimation.Param{
		ParamTotalDiskGB: {Key: ParamTotalDiskGB, Value: 500.0},
	}

	result, err := calc.Calculate(params)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// (500 GB * 1024 MB/GB) / (1600 Mbps / 8) / 60 s/min
	expectedMins := (500.0 * 1024.0) / (customRate / 8) / 60.0
	expectedDuration := time.Duration(expectedMins * float64(time.Minute))
	if result.Duration == nil {
		t.Fatal("expected point estimation, got ranged")
	}
	if *result.Duration != expectedDuration {
		t.Errorf("expected duration %v, got %v", expectedDuration, *result.Duration)
	}
}

func TestStorageMigration_Calculate_HighBandwidth(t *testing.T) {
	t.Parallel()
	// 10x the default rate should yield 1/10 the duration
	const highRate = DefaultTransferRateMbps * 10
	calc := NewStorageMigration(WithTransferRateMbps(highRate))

	params := map[string]estimation.Param{
		ParamTotalDiskGB: {Key: ParamTotalDiskGB, Value: 1000.0},
	}

	result, err := calc.Calculate(params)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	expectedMins := (1000.0 * 1024.0) / (highRate / 8) / 60.0
	expectedDuration := time.Duration(expectedMins * float64(time.Minute))
	if result.Duration == nil {
		t.Fatal("expected point estimation, got ranged")
	}
	if *result.Duration != expectedDuration {
		t.Errorf("expected duration %v, got %v", expectedDuration, *result.Duration)
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
	if result.Duration == nil {
		t.Fatal("expected point estimation, got ranged")
	}
	if *result.Duration != 0 {
		t.Errorf("expected 0 duration for zero GB, got %v", *result.Duration)
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
