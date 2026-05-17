package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

type uniqueVisits struct {
	counter       prometheus.Gauge
	visitorsCache map[string]struct{} // TODO: We may want to make this persistent in the DB.
	mu            sync.RWMutex
}

// Visits
const visitCountPerWeek = "visits_count_per_week"

var totalUniqueVisitPerWeekMetric = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Subsystem: assistedMigration,
		Name:      visitCountPerWeek,
		Help:      "metrics to record the number of unique visits per week",
	},
)

var UniqueVisitsPerWeek = &uniqueVisits{
	counter:       totalUniqueVisitPerWeekMetric,
	visitorsCache: make(map[string]struct{}),
}

func (v *uniqueVisits) Reset() {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.visitorsCache = make(map[string]struct{})
	v.counter.Set(0)
}

func (v *uniqueVisits) IncreaseTotalUniqueVisit(visitor string) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if _, exists := v.visitorsCache[visitor]; exists {
		return
	}

	v.visitorsCache[visitor] = struct{}{}
	v.counter.Inc()
}
