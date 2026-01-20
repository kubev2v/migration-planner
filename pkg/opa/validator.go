package opa

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kubev2v/forklift/pkg/controller/provider/model/vsphere"
	web "github.com/kubev2v/forklift/pkg/controller/provider/web/vsphere"
	"github.com/kubev2v/migration-planner/pkg/duckdb_parser/models"

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
		// Parse with Rego v1; ensure policies are v1-compatible
		module, err := ast.ParseModuleWithOpts(filename, content, ast.ParserOptions{
			RegoVersion: ast.RegoV1,
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
func (v *Validator) concerns(ctx context.Context, input interface{}) ([]vsphere.Concern, error) {
	resultSet, err := v.preparedQuery.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return nil, fmt.Errorf("policy evaluation failed: %w", err)
	}

	if len(resultSet) == 0 || len(resultSet[0].Expressions) == 0 {
		zap.S().Named("opa").Debug("No policy results returned")
		return []vsphere.Concern{}, nil
	}

	raw, ok := resultSet[0].Expressions[0].Value.([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected result type from policy evaluation")
	}

	// convert results to concern model
	var concerns []vsphere.Concern
	for _, r := range raw {
		m, ok := r.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("unexpected item type in result set")
		}

		b, err := json.Marshal(m)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal concern data: %w", err)
		}

		var c vsphere.Concern
		if err := json.Unmarshal(b, &c); err != nil {
			return nil, fmt.Errorf("failed to unmarshal concern: %w", err)
		}

		concerns = append(concerns, c)
	}

	return concerns, nil
}

func (v *Validator) ValidateVM(ctx context.Context, vm vsphere.VM) ([]vsphere.Concern, error) {
	// Prepare the JSON data in MTV OPA server format
	workload := web.Workload{}
	workload.With(&vm)

	concerns, err := v.concerns(ctx, workload)
	if err != nil {
		return nil, fmt.Errorf("failed to validate VM %q: %w", vm.Name, err)
	}

	return concerns, nil
}

// Validate implements duckdb_parser.Validator for models.VM.
// This allows OPA validation to work directly with duckdb_parser models.
func (v *Validator) Validate(ctx context.Context, vm models.VM) ([]models.Concern, error) {
	// models.VM is already JSON-compatible with the OPA input format,
	// so we can pass it directly for evaluation
	resultSet, err := v.preparedQuery.Eval(ctx, rego.EvalInput(vm))
	if err != nil {
		return nil, fmt.Errorf("policy evaluation failed for VM %q: %w", vm.Name, err)
	}

	var concerns []models.Concern

	if len(resultSet) > 0 && len(resultSet[0].Expressions) > 0 {
		raw, ok := resultSet[0].Expressions[0].Value.([]interface{})
		if !ok {
			return nil, fmt.Errorf("unexpected result type from policy evaluation")
		}

		for _, r := range raw {
			m, ok := r.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("unexpected item type in result set")
			}

			b, err := json.Marshal(m)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal concern data: %w", err)
			}

			var c models.Concern
			if err := json.Unmarshal(b, &c); err != nil {
				return nil, fmt.Errorf("failed to unmarshal concern: %w", err)
			}

			concerns = append(concerns, c)
		}
	}

	// Add OS upgrade recommendation for upgradable OSes
	if upgradeConcern := GetOSUpgradeConcern(vm.EffectiveGuestName()); upgradeConcern != nil {
		concerns = append(concerns, *upgradeConcern)
	}

	return concerns, nil
}
