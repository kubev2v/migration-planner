package calculators

import (
	"fmt"
	"time"

	"github.com/kubev2v/migration-planner/pkg/estimations/estimation"
)

const (
	storageOffloadMinRateGBps = 0.5
	storageOffloadMaxRateGBps = 2.0
)

// Compile-time assertion that StorageOffload implements the Calculator interface.
var _ estimation.Calculator = (*StorageOffload)(nil)

// StorageOffload estimates storage transfer time assuming x-copy is enabled
// and the same storage array is used, with a transfer rate of 0.5–2 GB/s.
type StorageOffload struct{}

func NewStorageOffload() *StorageOffload { return &StorageOffload{} }

func (c *StorageOffload) Name() string   { return "Storage Offload" }
func (c *StorageOffload) Keys() []string { return []string{ParamTotalDiskGB} }

func (c *StorageOffload) Calculate(params map[string]estimation.Param) (estimation.Estimation, error) {
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

	// fastest rate → shortest duration (Min); slowest rate → longest duration (Max)
	minSecs := totalGB / storageOffloadMaxRateGBps
	maxSecs := totalGB / storageOffloadMinRateGBps
	minDuration := time.Duration(minSecs * float64(time.Second))
	maxDuration := time.Duration(maxSecs * float64(time.Second))

	reason := fmt.Sprintf(
		"%.2f GB @ %.1f–%.1f GB/s transfer rate; assumes same storage array with x-copy enabled",
		totalGB, storageOffloadMinRateGBps, storageOffloadMaxRateGBps,
	)
	return estimation.NewRangedEstimation(minDuration, maxDuration, reason), nil
}
