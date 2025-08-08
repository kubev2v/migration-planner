package opa

import (
	"context"
	"fmt"

	"github.com/kubev2v/forklift/pkg/controller/provider/model/vsphere"
	web "github.com/kubev2v/forklift/pkg/controller/provider/web/vsphere"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/rego"
	"go.uber.org/zap"
)

// Validator handles policy compilation and validation
type Validator struct {
	preparedQuery rego.PreparedEvalQuery
}

func NewValidatorFromDir(policiesDir string) (*Validator, error) {
	reader := NewPolicyReader()

	policies, err := reader.ReadPolicies(policiesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read policies: %w", err)
	}

	return NewValidator(policies)
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

// compilePolicies Compile the provided policy content and prepares the query
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

// concerns Validate the provided input against compiled policies
func (v *Validator) concerns(ctx context.Context, input interface{}) ([]interface{}, error) {
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

func (v *Validator) ValidateVMs(ctx context.Context, vms []vsphere.VM) ([]vsphere.VM, error) {

	var validationErrors []error

	validatedVMs := make([]vsphere.VM, 0, len(vms))

	for _, vm := range vms {
		// Prepare the JSON data in MTV OPA server format
		workload := web.Workload{}
		workload.With(&vm)

		concerns, err := v.concerns(ctx, workload)
		if err != nil {
			validationErrors = append(validationErrors, fmt.Errorf("failed to validate VM %q: %w", vm.Name, err))
			continue
		}

		// Convert concerns to vsphere.Concern format
		for _, c := range concerns {
			concernMap, ok := c.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf(
					"unexpected concern data type for VM %q: got %T, expected map[string]interface{}",
					vm.Name,
					c,
				)
			}

			concern := vsphere.Concern{}
			if id, ok := concernMap["id"].(string); ok {
				concern.Id = id
			}

			if label, ok := concernMap["label"].(string); ok {
				concern.Label = label
			}

			if assessment, ok := concernMap["assessment"].(string); ok {
				concern.Assessment = assessment
			}

			if category, ok := concernMap["category"].(string); ok {
				concern.Category = category
			}

			vm.Concerns = append(vm.Concerns, concern)
		}

		validatedVMs = append(validatedVMs, vm)
	}

	if len(validationErrors) > 0 {
		return validatedVMs, fmt.Errorf("validation completed with %d error(s): %v", len(validationErrors), validationErrors)
	}

	return validatedVMs, nil
}
