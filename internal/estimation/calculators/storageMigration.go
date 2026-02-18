package calculators

import (
	"fmt"
	"time"

	"github.com/kubev2v/migration-planner/internal/estimation"
)

const (
	// ParamTotalDiskGB is the estimation.Param key for the total disk size across all VMs in gigabytes.
	ParamTotalDiskGB = "total_disk_gb"
	// DefaultMinutesPer500GB is the default rate used when no override is provided.
	DefaultMinutesPer500GB = 110.0
)

// Compile-time assertion that StorageMigration implements the Calculator interface.
var _ estimation.Calculator = (*StorageMigration)(nil)

// StorageMigration estimates the time required to transfer VM storage data from the source to the target cluster.
type StorageMigration struct {
	minutesPer500GB float64
}

// StorageMigrationOption is a functional option for configuring a StorageMigration calculator.
type StorageMigrationOption func(*StorageMigration)

// WithMinutesPer500GB sets the number of minutes required to migrate 500 GB of storage.
// The value must be positive; non-positive values are ignored and the default is kept.
func WithMinutesPer500GB(minutesPer500GB float64) StorageMigrationOption {
	return func(storageMigration *StorageMigration) {
		if minutesPer500GB > 0 {
			storageMigration.minutesPer500GB = minutesPer500GB
		}
	}
}

// NewStorageMigration creates a StorageMigration calculator with default settings.
// Optional StorageMigrationOption values can be supplied to override the defaults.
func NewStorageMigration(opts ...StorageMigrationOption) *StorageMigration {
	res := StorageMigration{
		minutesPer500GB: DefaultMinutesPer500GB,
	}

	for _, opt := range opts {
		opt(&res)
	}

	return &res
}

// Name returns the human-readable name of this calculator.
func (c *StorageMigration) Name() string {
	return "Storage Migration"
}

// Keys returns the list of parameter keys required by this calculator.
func (c *StorageMigration) Keys() []string {
	return []string{ParamTotalDiskGB}
}

// Calculate estimates the storage migration duration based on total disk size.
// It requires the ParamTotalDiskGB parameter to be present in params.
func (c *StorageMigration) Calculate(params map[string]estimation.Param) (estimation.Estimation, error) {
	// Extract and validate total disk GB
	diskParam, ok := params[ParamTotalDiskGB]
	if !ok {
		return estimation.Estimation{}, fmt.Errorf("missing %s", ParamTotalDiskGB)
	}

	totalGB, err := getFloat(diskParam)
	if err != nil {
		return estimation.Estimation{}, err
	}

	if totalGB < 0 {
		return estimation.Estimation{}, fmt.Errorf("%s must be non-negative", ParamTotalDiskGB)
	}

	// Calculate: (totalGB / 500) * minutesPer500GB
	units := totalGB / 500.0
	totalMinutes := units * c.minutesPer500GB
	duration := time.Duration(totalMinutes * float64(time.Minute))

	return estimation.Estimation{
		Duration: duration,
		Reason:   fmt.Sprintf("%.2f GB at %.0f minutes per 500GB", totalGB, c.minutesPer500GB),
	}, nil
}
