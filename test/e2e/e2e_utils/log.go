package e2e_utils

import (
	. "github.com/kubev2v/migration-planner/test/e2e"
	"go.uber.org/zap"
	"sort"
	"time"
)

// LogExecutionSummary logs the execution time of all tests stored in the TestsExecutionTime map.
// It sorts the tests by duration in descending order and logs the test name along with its execution duration.
func LogExecutionSummary() {
	zap.S().Infof("============Summarizing execution time============")

	type testResult struct {
		name     string
		duration time.Duration
	}

	var results []testResult

	for test, duration := range TestsExecutionTime {
		results = append(results, testResult{name: test, duration: duration})
	}

	// Sort tests by duration descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].duration > results[j].duration
	})

	for _, result := range results {
		zap.S().Infof("[%s] finished after: %s", result.name, result.duration.Round(time.Second))
	}
}
