package opa

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testPolicy = `package io.konveyor.forklift.vmware
import future.keywords.in

concerns[flag] {
    input.name == "test-vm-with-concern"
    flag := {
        "id": "test.simple.concern",
        "category": "Warning", 
        "label": "Test VM detected",
        "assessment": "This is a test concern."
    }
}
`

func TestNewPolicyReader(t *testing.T) {
	reader := NewPolicyReader()
	if reader == nil {
		t.Error("NewPolicyReader() returned nil")
	}
}

func TestPolicyReader_DiscoverPoliciesDirectory(t *testing.T) {
	originalEnv := os.Getenv("OPA_POLICIES_DIR")
	defer func() {
		if originalEnv != "" {
			os.Setenv("OPA_POLICIES_DIR", originalEnv)
		} else {
			os.Unsetenv("OPA_POLICIES_DIR")
		}
	}()

	reader := NewPolicyReader()

	// Test with environment variable set to existing directory
	dir := t.TempDir()
	policyFile := filepath.Join(dir, "test.rego")
	if err := os.WriteFile(policyFile, []byte(testPolicy), 0644); err != nil {
		t.Fatalf("Failed to write test policy: %v", err)
	}

	os.Setenv("OPA_POLICIES_DIR", dir)
	result := reader.DiscoverPoliciesDirectory()
	if result != dir {
		t.Errorf("DiscoverPoliciesDirectory() should return env var directory, got: %s", result)
	}

	// Test with environment variable set to non-existent directory
	os.Setenv("OPA_POLICIES_DIR", "/non/existent/path")
	result = reader.DiscoverPoliciesDirectory()
	if result != "" {
		t.Errorf("DiscoverPoliciesDirectory() should return empty string for non-existent directory, got: %s", result)
	}

	// Test with no environment variable (should use default)
	os.Unsetenv("OPA_POLICIES_DIR")
	result = reader.DiscoverPoliciesDirectory()
	if result != "" {
		t.Errorf("DiscoverPoliciesDirectory() should return empty string for default non-existent directory, got: %s", result)
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
	if !strings.Contains(err.Error(), "policies directory does not exist") {
		t.Errorf("ReadPolicies() error should mention missing directory, got: %v", err)
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

func TestValidator_ValidateConcerns_WithConcern(t *testing.T) {
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

	concerns, err := validator.ValidateConcerns(context.Background(), vmInput)
	if err != nil {
		t.Fatalf("ValidateConcerns() failed: %v", err)
	}

	if len(concerns) != 1 {
		t.Errorf("Expected 1 concern, got %d", len(concerns))
	}
}

func TestValidator_ValidateConcerns_WithoutConcern(t *testing.T) {
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

	concerns, err := validator.ValidateConcerns(context.Background(), vmInput)
	if err != nil {
		t.Fatalf("ValidateConcerns() failed: %v", err)
	}

	if len(concerns) != 0 {
		t.Errorf("Expected 0 concerns, got %d", len(concerns))
	}
}

func TestSetupValidator(t *testing.T) {
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

	os.Setenv("OPA_POLICIES_DIR", dir)

	validator, err := SetupValidator()
	if err != nil {
		t.Fatalf("SetupValidator() failed: %v", err)
	}

	if validator == nil {
		t.Error("SetupValidator() returned nil validator")
	}
}

// Test helper functions

func TestIsPoliciesDirectory(t *testing.T) {
	// Test with directory containing .rego files
	dir := t.TempDir()
	policyFile := filepath.Join(dir, "test.rego")
	if err := os.WriteFile(policyFile, []byte(testPolicy), 0644); err != nil {
		t.Fatalf("Failed to write test policy: %v", err)
	}

	if !isPoliciesDirectory(dir) {
		t.Error("isPoliciesDirectory() should return true for directory with .rego files")
	}

	// Test with non-existent directory
	if isPoliciesDirectory("/non/existent/path") {
		t.Error("isPoliciesDirectory() should return false for non-existent directory")
	}
}
