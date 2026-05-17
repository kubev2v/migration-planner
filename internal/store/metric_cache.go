package store

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/kubev2v/migration-planner/internal/store/model"
	"golang.org/x/sync/singleflight"
)

const (
	minCooldownPeriod = 5 * time.Minute
	maxCooldownPeriod = 3 * time.Hour
)

// MetricsCache manages cached inventory statistics
type MetricsCache struct {
	assessmentStore Assessment
	needsUpdate     atomic.Bool // Set when the server modifies data requiring cache refresh

	stats       atomic.Pointer[model.InventoryStats]
	lastRefresh atomic.Int64

	group singleflight.Group
}

// NewMetricsCache creates a new metrics cache
func NewMetricsCache(s Assessment) *MetricsCache {
	return &MetricsCache{
		assessmentStore: s,
	}
}

// GetStats returns cached stats, refreshing only if cooldown expired.
func (mc *MetricsCache) GetStats(ctx context.Context) (model.InventoryStats, error) {
	ptr := mc.stats.Load()

	if ptr != nil && !mc.shouldRefresh() {
		return *ptr, nil
	}

	v, err, _ := mc.group.Do("refresh_stats", func() (any, error) {

		assessments, err := mc.assessmentStore.List(ctx, NewAssessmentQueryFilter())
		if err != nil {
			return nil, err
		}

		stats := model.NewInventoryStats(assessments)

		mc.stats.Store(&stats)
		mc.lastRefresh.Store(time.Now().UnixNano())
		mc.needsUpdate.Store(false)

		return stats, nil
	})

	if err != nil {
		return model.InventoryStats{}, fmt.Errorf("refresh cache failed: %w", err)
	}

	return v.(model.InventoryStats), nil
}

func (mc *MetricsCache) RequestMetricsCacheRefresh() {
	mc.needsUpdate.Store(true)
}

// shouldRefresh checks if cooldown period has passed
func (mc *MetricsCache) shouldRefresh() bool {
	last := mc.lastRefresh.Load()
	if last == 0 {
		return true
	}

	// Potential change by other pods
	if time.Since(time.Unix(0, last)) > maxCooldownPeriod {
		return true
	}

	if !mc.needsUpdate.Load() {
		return false
	}

	return time.Since(time.Unix(0, last)) > minCooldownPeriod
}
