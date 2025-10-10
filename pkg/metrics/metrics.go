package metrics

import (
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	assistedMigration = "assisted_migration"

	// Ova metrics
	ovaDownloadsTotal = "ova_downloads_total"

	// Agent metrics
	AgentStatusCount = "agent_status_count"

	// Authz metrics
	authzTotalRelationships = "authz_total_relationships"
	authzValidRelationships = "authz_valid_relationships"

	// Labels
	agentStateLabel        = "state"
	ovaDownloadStatusLabel = "state"
)

var agentStateCountLabels = []string{
	agentStateLabel,
}

var ovaDownloadTotalLabels = []string{
	ovaDownloadStatusLabel,
}

/**
* Metrics definition
**/
var ovaDownloadsTotalMetric = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Subsystem: assistedMigration,
		Name:      ovaDownloadsTotal,
		Help:      "number of total ova downloads",
	},
	ovaDownloadTotalLabels,
)

var agentStatusCountMetric = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Subsystem: assistedMigration,
		Name:      AgentStatusCount,
		Help:      "metrics to record the number of agents in each status",
	},
	agentStateCountLabels,
)

var authzTotalRelationshipsMetric = prometheus.NewCounter(
	prometheus.CounterOpts{
		Subsystem: assistedMigration,
		Name:      authzTotalRelationships,
		Help:      "total number of authz relationships computed",
	},
)

var authzValidRelationshipsMetric = prometheus.NewCounter(
	prometheus.CounterOpts{
		Subsystem: assistedMigration,
		Name:      authzValidRelationships,
		Help:      "total number of authz relationships written successfully",
	},
)

func IncreaseOvaDownloadsTotalMetric(state string) {
	labels := prometheus.Labels{
		ovaDownloadStatusLabel: state,
	}
	ovaDownloadsTotalMetric.With(labels).Inc()
}

func UpdateAgentStateCounterMetric(state string, count int) {
	labels := prometheus.Labels{
		agentStateLabel: state,
	}
	agentStatusCountMetric.With(labels).Set(float64(count))
}

func IncreaseAuthzTotalRelationshipsMetric(count int) {
	authzTotalRelationshipsMetric.Add(float64(count))
}

func IncreaseAuthzValidRelationshipsMetric(count int) {
	authzValidRelationshipsMetric.Add(float64(count))
}

func RegisterMetrics(s store.Store) {
	inventoryStatsCollector := newInventoryStatsCollector(s)

	prometheus.MustRegister(inventoryStatsCollector)
	prometheus.MustRegister(ovaDownloadsTotalMetric)
	prometheus.MustRegister(agentStatusCountMetric)
	prometheus.MustRegister(authzTotalRelationshipsMetric)
	prometheus.MustRegister(authzValidRelationshipsMetric)
}
