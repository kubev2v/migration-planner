package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	assistedMigration = "assisted_migration"

	// Ova metrics
	ovaDownloadsTotal = "ova_downloads_total"

	// Agent metrics
	AgentStatusCount = "agent_status_count"

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

func init() {
	registerMetrics()
}

func registerMetrics() {
	prometheus.MustRegister(ovaDownloadsTotalMetric)
	prometheus.MustRegister(agentStatusCountMetric)
}
