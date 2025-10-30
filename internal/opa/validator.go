package opa

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"

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

func (v *Validator) ValidateVMs(ctx context.Context, vms *[]vsphere.VM) error {
	if vms == nil || len(*vms) == 0 {
		return nil
	}

	// Use worker pool for parallel validation
	numWorkers := runtime.NumCPU() * 2
	if numWorkers > len(*vms) {
		numWorkers = len(*vms)
	}

	// Channel for distributing work
	type vmJob struct {
		index int
		vm    *vsphere.VM
	}
	jobs := make(chan vmJob, len(*vms))

	// Shared state for collecting errors (protected by mutex)
	var errorsMu sync.Mutex
	var validationErrors []error

	// WaitGroup to wait for all workers to finish
	var wg sync.WaitGroup

	// Start worker goroutines
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for job := range jobs {
				// Check if context was canceled
				select {
				case <-ctx.Done():
					errorsMu.Lock()
					validationErrors = append(validationErrors,
						fmt.Errorf("validation canceled: %w", ctx.Err()))
					errorsMu.Unlock()
					return
				default:
				}

				vm := job.vm

				// Prepare the JSON data in MTV OPA server format
				workload := web.Workload{}
				workload.With(vm)

				concerns, err := v.concerns(ctx, workload)
				if err != nil {
					errorsMu.Lock()
					validationErrors = append(validationErrors,
						fmt.Errorf("failed to validate VM %q: %w", vm.Name, err))
					errorsMu.Unlock()
					continue
				}

				// Convert concerns to vsphere.Concern format
				for _, c := range concerns {
					concernMap, ok := c.(map[string]interface{})
					if !ok {
						errorsMu.Lock()
						validationErrors = append(validationErrors, fmt.Errorf(
							"unexpected concern data type for VM %q: got %T, expected map[string]interface{}",
							vm.Name,
							c,
						))
						errorsMu.Unlock()
						continue
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

					// Append concern to VM (safe because each worker processes different VMs)
					vm.Concerns = append(vm.Concerns, concern)
				}
			}
		}(w)
	}

	// Send all VMs to the jobs channel
	for i := range *vms {
		jobs <- vmJob{
			index: i,
			vm:    &(*vms)[i],
		}
	}
	close(jobs) // No more jobs to send

	// Wait for all workers to finish
	wg.Wait()

	if len(validationErrors) > 0 {
		return fmt.Errorf("validation completed with %d error(s): %w", len(validationErrors), errors.Join(validationErrors...))
	}

	return nil
}
