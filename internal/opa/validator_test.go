package opa

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kubev2v/forklift/pkg/controller/provider/model/vsphere"
)

const testPolicy = `package io.konveyor.forklift.vmware

import rego.v1

concerns contains flag if {
	input.name == "test-vm-with-concern"
	flag := {
		"id": "test.simple.concern",
		"category": "Warning",
		"label": "Test VM detected",
		"assessment": "This is a test concern.",
	}
}

concerns contains flag if {
	input.guestId == "rhel6guest"
	flag := {
		"id": "test.simple.concern",
		"category": "Warning",
		"label": "Test VM detected",
		"assessment": "This is a test concern.",
	}
}
`

func TestNewPolicyReader(t *testing.T) {
	reader := NewPolicyReader()
	if reader == nil {
		t.Error("NewPolicyReader() returned nil")
	}
}

func TestPolicyReader_ReadPolicies_Success(t *testing.T) {
	// Create temp directory with test policy
	dir := t.TempDir()
	policyFile := filepath.Join(dir, "test.rego")
	if err := os.WriteFile(policyFile, []byte(testPolicy), 0644); err != nil {
		t.Fatalf("Failed to write test policy: %v", err)
	}

	reader := NewPolicyReader()
	policies, err := reader.ReadPolicies(dir)
	if err != nil {
		t.Fatalf("ReadPolicies() failed: %v", err)
	}

	if len(policies) != 1 {
		t.Errorf("Expected 1 policy, got %d", len(policies))
	}

	if content, exists := policies["test.rego"]; !exists {
		t.Error("Expected test.rego policy not found")
	} else if content != testPolicy {
		t.Error("Policy content doesn't match expected content")
	}
}

func TestPolicyReader_ReadPolicies_NonExistentDirectory(t *testing.T) {
	reader := NewPolicyReader()
	_, err := reader.ReadPolicies("/non/existent/path")
	if err == nil {
		t.Error("ReadPolicies() expected error for non-existent directory")
	}
}

func TestPolicyReader_ReadPolicies_SkipsTestFiles(t *testing.T) {
	// Create temp directory with policy and test files
	dir := t.TempDir()

	// Regular policy file
	policyFile := filepath.Join(dir, "policy.rego")
	if err := os.WriteFile(policyFile, []byte(testPolicy), 0644); err != nil {
		t.Fatalf("Failed to write policy file: %v", err)
	}

	// Test file (should be skipped)
	testFile := filepath.Join(dir, "policy_test.rego")
	if err := os.WriteFile(testFile, []byte("# test file"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	reader := NewPolicyReader()
	policies, err := reader.ReadPolicies(dir)
	if err != nil {
		t.Fatalf("ReadPolicies() failed: %v", err)
	}

	if len(policies) != 1 {
		t.Errorf("Expected 1 policy (test files should be skipped), got %d", len(policies))
	}

	if _, exists := policies["policy_test.rego"]; exists {
		t.Error("Test file should have been skipped")
	}
}

// Test Validator

func TestNewValidator_Success(t *testing.T) {
	policies := map[string]string{
		"test.rego": testPolicy,
	}

	validator, err := NewValidator(policies)
	if err != nil {
		t.Fatalf("NewValidator() failed: %v", err)
	}

	if validator == nil {
		t.Error("NewValidator() returned nil validator")
	}
}

func TestNewValidator_EmptyPolicies(t *testing.T) {
	_, err := NewValidator(map[string]string{})
	if err == nil {
		t.Error("NewValidator() expected error for empty policies")
	}
	if !strings.Contains(err.Error(), "no policies provided") {
		t.Errorf("NewValidator() error should mention no policies, got: %v", err)
	}
}

func TestValidator_Concerns_WithConcern(t *testing.T) {
	policies := map[string]string{
		"test.rego": testPolicy,
	}

	validator, err := NewValidator(policies)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	// Test VM that should trigger concern
	vmInput := map[string]interface{}{
		"name": "test-vm-with-concern",
	}

	concerns, err := validator.concerns(context.Background(), vmInput)
	if err != nil {
		t.Fatalf("concerns() failed: %v", err)
	}

	if len(concerns) != 1 {
		t.Errorf("Expected 1 concern, got %d", len(concerns))
	}
}

func TestValidator_ValidateVMS(t *testing.T) {
	policies := map[string]string{
		"test.rego": testPolicy,
	}

	validator, err := NewValidator(policies)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	var vms []vsphere.VM
	vms = append(vms, vsphere.VM{
		GuestID: "rhel6guest",
	})

	for i := range vms {
		concerns, err := validator.ValidateVM(context.Background(), vms[i])
		if err != nil {
			t.Fatalf("ValidateVM() failed: %v", err)
		}
		vms[i].Concerns = concerns
	}

	if len(vms) != 1 {
		t.Errorf("Expected 1 vm, got %d", len(vms))
	}

	if len(vms[0].Concerns) != 1 {
		t.Errorf("Expected 1 concern, got %d", len(vms[0].Concerns))
	}
}

func TestValidator_Concerns_WithoutConcern(t *testing.T) {
	policies := map[string]string{
		"test.rego": testPolicy,
	}

	validator, err := NewValidator(policies)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	// Test VM that should NOT trigger concern
	vmInput := map[string]interface{}{
		"name": "clean-vm",
	}

	concerns, err := validator.concerns(context.Background(), vmInput)
	if err != nil {
		t.Fatalf("concerns() failed: %v", err)
	}

	if len(concerns) != 0 {
		t.Errorf("Expected 0 concerns, got %d", len(concerns))
	}
}

func TestNewValidatorFromDir(t *testing.T) {
	// Save original environment
	originalEnv := os.Getenv("OPA_POLICIES_DIR")
	defer func() {
		if originalEnv != "" {
			os.Setenv("OPA_POLICIES_DIR", originalEnv)
		} else {
			os.Unsetenv("OPA_POLICIES_DIR")
		}
	}()

	// Create temp directory with test policy
	dir := t.TempDir()
	policyFile := filepath.Join(dir, "test.rego")
	if err := os.WriteFile(policyFile, []byte(testPolicy), 0644); err != nil {
		t.Fatalf("Failed to write test policy: %v", err)
	}

	validator, err := NewValidatorFromDir(dir)
	if err != nil {
		t.Fatalf("NewValidatorFromDir() failed: %v", err)
	}

	if validator == nil {
		t.Error("NewValidatorFromDir() returned nil validator")
	}
}
