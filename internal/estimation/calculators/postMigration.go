package calculators

import (
	"fmt"
	"time"

	"github.com/kubev2v/migration-planner/internal/estimation"
)

// Param prefix = parameter keys in the params map given to the calculator
// Default prefix = default values for missing params and/or hardcoded assumptions
const (

	// ParamVMCount total number of VMs in a cluster.
	ParamVMCount = "vm_count"
	// ParamTroubleshootMinsPerVM troubleshooting time in minutes per vm
	ParamTroubleshootMinsPerVM = "troubleshoot_mins_per_vm"
	// ParamPostMigrationEngineers number of engineers available to perform post-migration checks
	ParamPostMigrationEngineers = "post_migration_engineers"

	DefaultTroubleshootMinsPerVM = 60.0
	DefaultEngineerCount         = 10
)

// Compile-time assertion that PostMigrationTroubleShooting implements the Calculator interface.
var _ estimation.Calculator = (*PostMigrationTroubleShooting)(nil)

// PostMigrationTroubleShooting estimates time for post migration actions done by an engineers
type PostMigrationTroubleShooting struct {
	troubleshootMinsPerVM float64
	engineerCount         int
}

// PostMigrationTroubleshootingOption configuration option for the calculator
type PostMigrationTroubleshootingOption func(*PostMigrationTroubleShooting)

// WithTroubleshootMinsPerVM sets the number of minutes spent troubleshooting each VM after migration.
func WithTroubleshootMinsPerVM(mins float64) PostMigrationTroubleshootingOption {
	return func(p *PostMigrationTroubleShooting) {
		p.troubleshootMinsPerVM = mins
	}
}

// WithEngineerCount sets the number of engineers working in parallel during post-migration checks.
func WithEngineerCount(count int) PostMigrationTroubleshootingOption {
	return func(p *PostMigrationTroubleShooting) {
		p.engineerCount = count
	}
}

// NewPostMigrationTroubleShooting creates a PostMigrationTroubleShooting calculator with default settings that
//
//	can be overridden by Options
func NewPostMigrationTroubleShooting(opts ...PostMigrationTroubleshootingOption) *PostMigrationTroubleShooting {
	res := PostMigrationTroubleShooting{
		troubleshootMinsPerVM: DefaultTroubleshootMinsPerVM,
		engineerCount:         DefaultEngineerCount,
	}

	for _, opt := range opts {
		opt(&res)
	}

	return &res
}

// Name returns the human-readable name of this calculator.
func (c *PostMigrationTroubleShooting) Name() string { return "Post-Migration Checks" }

// Keys returns the list of parameter keys required by this calculator.
func (c *PostMigrationTroubleShooting) Keys() []string {
	return []string{ParamVMCount}
}

// Calculate estimates the post-migration troubleshooting duration based on VM count and engineer availability.
// ParamTroubleshootMinsPerVM and ParamPostMigrationEngineers are optional and fall back to the struct defaults.
func (c *PostMigrationTroubleShooting) Calculate(params map[string]estimation.Param) (estimation.Estimation, error) {
	// Extract VM count (required)
	vmParam, ok := params[ParamVMCount]
	if !ok {
		return estimation.Estimation{}, fmt.Errorf("missing %s", ParamVMCount)
	}
	vmCount, err := getInt(vmParam)
	if err != nil {
		return estimation.Estimation{}, err
	}

	if vmCount < 0 {
		return estimation.Estimation{}, fmt.Errorf("vm_count must be non-negative")
	}

	// Extract mins per VM (optional - falls back to struct field/default)
	minsPerVM := c.troubleshootMinsPerVM
	if timeParam, exists := params[ParamTroubleshootMinsPerVM]; exists {
		paramMins, err := getFloat(timeParam)
		if err != nil {
			return estimation.Estimation{}, err
		}
		minsPerVM = paramMins
	}

	// Extract engineer count (optional - falls back to struct field/default)
	engineerCount := c.engineerCount
	if engParam, exists := params[ParamPostMigrationEngineers]; exists {
		paramEngineers, err := getInt(engParam)
		if err != nil {
			return estimation.Estimation{}, err
		}
		engineerCount = paramEngineers
	}

	if engineerCount <= 0 {
		return estimation.Estimation{}, fmt.Errorf("engineers must be > 0")
	}

	// Calculate total man-minutes and divide by engineers
	totalManMins := float64(vmCount) * minsPerVM
	realTimeMins := totalManMins / float64(engineerCount)

	return estimation.Estimation{
		Duration: time.Duration(realTimeMins * float64(time.Minute)),
		Reason:   fmt.Sprintf("%d VMs @ %.1f mins each / %d engineers", vmCount, minsPerVM, engineerCount),
	}, nil
}
