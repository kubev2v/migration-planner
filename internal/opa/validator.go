package opa

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kubev2v/migration-planner/internal/util"
	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/rego"
	"go.uber.org/zap"
)

// Handle policy discovery and file reading
type PolicyReader struct{}

func NewPolicyReader() *PolicyReader {
	return &PolicyReader{}
}

// Find the policies directory using OPA_POLICIES_DIR environment variable
func (pr *PolicyReader) DiscoverPoliciesDirectory() string {
	logger := zap.S().Named("opa")

	policiesDir := util.GetEnv("OPA_POLICIES_DIR", "/app/policies")

	if isPoliciesDirectory(policiesDir) {
		logger.Infof("Found policies directory: %s", policiesDir)
		return policiesDir
	}

	logger.Warnf("No OPA policies found in: %s", policiesDir)
	logger.Info("To set up policies, run: make setup-opa-policies")
	logger.Info("Or set OPA_POLICIES_DIR environment variable to custom location")
	return ""
}

// Read all .rego policy files from the specified directory
func (pr *PolicyReader) ReadPolicies(policiesDir string) (map[string]string, error) {
	if !isPoliciesDirectory(policiesDir) {
		return nil, fmt.Errorf("policies directory does not exist or contains no .rego files: %s", policiesDir)
	}

	policies := make(map[string]string)

	entries, err := os.ReadDir(policiesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read policies directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".rego") ||
			strings.HasSuffix(entry.Name(), "_test.rego") {
			continue // Skip test files
		}

		path := filepath.Join(policiesDir, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read policy file %s: %w", path, err)
		}

		policies[entry.Name()] = string(content)
		zap.S().Named("opa").Debugf("Read policy: %s", entry.Name())
	}

	if len(policies) == 0 {
		return nil, fmt.Errorf("no .rego policy files found in directory: %s", policiesDir)
	}

	zap.S().Named("opa").Infof("Successfully read %d policy files from: %s", len(policies), policiesDir)
	return policies, nil
}

// Validator handles policy compilation and validation
type Validator struct {
	preparedQuery rego.PreparedEvalQuery
}

func NewValidator(policies map[string]string) (*Validator, error) {
	if len(policies) == 0 {
		return nil, fmt.Errorf("no policies provided for validation")
	}

	validator := &Validator{}

	if err := validator.compilePolicies(policies); err != nil {
		return nil, fmt.Errorf("failed to compile policies: %w", err)
	}

	zap.S().Named("opa").Infof("OPA validator initialized with %d policies", len(policies))
	return validator, nil
}

// Compile the provided policy content and prepares the query
func (v *Validator) compilePolicies(policies map[string]string) error {
	compiler := ast.NewCompiler()
	modules := make(map[string]*ast.Module)

	for filename, content := range policies {
		// Parse with v0 for compatibility with existing Forklift policies
		module, err := ast.ParseModuleWithOpts(filename, content, ast.ParserOptions{
			RegoVersion: ast.RegoV0,
		})
		if err != nil {
			return fmt.Errorf("failed to parse policy %s: %w", filename, err)
		}
		modules[filename] = module
	}

	compiler.Compile(modules)
	if compiler.Failed() {
		return fmt.Errorf("policy compilation failed: %v", compiler.Errors)
	}

	// Use v1 runtime for future-proofing and better performance
	r := rego.New(
		rego.Query("data.io.konveyor.forklift.vmware.concerns"),
		rego.Compiler(compiler),
		rego.SetRegoVersion(ast.RegoV1),
	)

	preparedQuery, err := r.PrepareForEval(context.Background())
	if err != nil {
		return fmt.Errorf("failed to prepare rego query: %w", err)
	}

	v.preparedQuery = preparedQuery
	zap.S().Named("opa").Infof("Successfully compiled %d policy files", len(policies))
	return nil
}

// Validate the provided input against compiled policies
func (v *Validator) ValidateConcerns(ctx context.Context, input interface{}) ([]interface{}, error) {
	resultSet, err := v.preparedQuery.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return nil, fmt.Errorf("policy evaluation failed: %w", err)
	}

	if len(resultSet) == 0 || len(resultSet[0].Expressions) == 0 {
		zap.S().Named("opa").Debug("No policy results returned")
		return []interface{}{}, nil
	}

	result, ok := resultSet[0].Expressions[0].Value.([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected result type from policy evaluation")
	}

	return result, nil
}

func SetupValidator() (*Validator, error) {
	reader := NewPolicyReader()
	policiesDir := reader.DiscoverPoliciesDirectory()
	if policiesDir == "" {
		return nil, fmt.Errorf("no policies directory found")
	}

	policies, err := reader.ReadPolicies(policiesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read policies: %w", err)
	}

	return NewValidator(policies)
}

func isPoliciesDirectory(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".rego") {
			return true // Return early on first .rego file found
		}
	}
	return false
}
