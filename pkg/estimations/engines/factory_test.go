package engines_test

import (
	"testing"

	"github.com/kubev2v/migration-planner/pkg/estimations/engines"
)

func TestBuildEngines_AllSchemas_WhenNilInput(t *testing.T) {
	t.Parallel()
	result, err := engines.BuildEngines(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 engines (all schemas), got %d", len(result))
	}
	if _, ok := result[engines.SchemaNetworkBased]; !ok {
		t.Error("expected SchemaNetworkBased engine")
	}
	if _, ok := result[engines.SchemaStorageOffload]; !ok {
		t.Error("expected SchemaStorageOffload engine")
	}
}

func TestBuildEngines_SingleSchema(t *testing.T) {
	t.Parallel()
	result, err := engines.BuildEngines([]engines.Schema{engines.SchemaNetworkBased})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 engine, got %d", len(result))
	}
}

func TestBuildEngines_UnknownSchema_ReturnsError(t *testing.T) {
	t.Parallel()
	_, err := engines.BuildEngines([]engines.Schema{"bogus-schema"})
	if err == nil {
		t.Error("expected error for unknown schema, got nil")
	}
}

func TestBuildEngines_EnginesAreIndependent(t *testing.T) {
	t.Parallel()
	result, err := engines.BuildEngines(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	nb := result[engines.SchemaNetworkBased]
	so := result[engines.SchemaStorageOffload]
	if nb == nil || so == nil {
		t.Fatal("expected non-nil engines")
	}
	if nb == so {
		t.Error("expected distinct engine instances")
	}
}
