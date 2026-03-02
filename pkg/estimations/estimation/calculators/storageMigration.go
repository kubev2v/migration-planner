package calculators

import (
	"fmt"
	"time"

	"github.com/kubev2v/migration-planner/pkg/estimations/estimation"
)

const (
	// ParamTotalDiskGB is the estimation.Param key for the total disk size across all VMs in gigabytes.
	ParamTotalDiskGB = "total_disk_gb"
	// ParamTransferRateMbps is the estimation.Param key for the sustained network transfer rate in Mbps.
	ParamTransferRateMbps = "transfer_rate_mbps"
	// DefaultTransferRateMbps is the default transfer rate in Mbps (megabits per second).
	// 620 Mbps is equivalent to 77.6 MB/s, which matches the original 110 min/500 GB baseline.
	DefaultTransferRateMbps = 620.0
)

// Compile-time assertion that StorageMigration implements the Calculator interface.
var _ estimation.Calculator = (*StorageMigration)(nil)

// StorageMigration estimates the time required to transfer VM storage data from the source to the target cluster.
type StorageMigration struct {
	transferRateMbps float64
}

// StorageMigrationOption is a functional option for configuring a StorageMigration calculator.
type StorageMigrationOption func(*StorageMigration)

// WithTransferRateMbps sets the sustained network transfer rate in Mbps used to estimate storage migration time.
// The value must be positive; non-positive values are ignored and the default is kept.
func WithTransferRateMbps(mbps float64) StorageMigrationOption {
	return func(s *StorageMigration) {
		if mbps > 0 {
			s.transferRateMbps = mbps
		}
	}
}

// NewStorageMigration creates a StorageMigration calculator with default settings.
// Optional StorageMigrationOption values can be supplied to override the defaults.
func NewStorageMigration(opts ...StorageMigrationOption) *StorageMigration {
	res := StorageMigration{
		transferRateMbps: DefaultTransferRateMbps,
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
// transfer_rate_mbps is optional and falls back to the struct default.
func (c *StorageMigration) Keys() []string {
	return []string{ParamTotalDiskGB}
}

// Calculate estimates the storage migration duration based on total disk size and network transfer rate.
// Formula: (totalDiskGB * 1024) / (transferRateMbps / 8) / 60
// transfer_rate_mbps is optional and falls back to the struct field default.
func (c *StorageMigration) Calculate(params map[string]estimation.Param) (estimation.Estimation, error) {
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

	transferRateMbps := c.transferRateMbps
	if rateParam, exists := params[ParamTransferRateMbps]; exists {
		paramRate, err := getFloat(rateParam)
		if err != nil {
			return estimation.Estimation{}, err
		}
		if paramRate > 0 {
			transferRateMbps = paramRate
		}
	}

	transferRateMBps := transferRateMbps / 8
	totalMinutes := (totalGB * 1024) / transferRateMBps / 60
	minsPer500GB := (500.0 * 1024.0) / transferRateMBps / 60.0
	duration := time.Duration(totalMinutes * float64(time.Minute))

	return estimation.Estimation{
		Duration: duration,
		Reason:   fmt.Sprintf("%.2f GB at %.0f Mbps (%.0f min/500GB)", totalGB, transferRateMbps, minsPer500GB),
	}, nil
}
