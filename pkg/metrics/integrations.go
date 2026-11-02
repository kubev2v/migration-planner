package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	IntegrationResultSuccess = "success"
	IntegrationResultFailure = "fail"
)

var integrationAttempts = prometheus.NewCounterVec(prometheus.CounterOpts{
	Subsystem: "assisted_migrations",
	Name:      "integration_attempts_total",
	Help:      "Number of completed integration calls or deliveries by final result. retries are not counted separately.",
}, []string{"integration", "result"})

func init() {
	prometheus.MustRegister(integrationAttempts)
}

// RecordIntegrationResult records a final outcome using metadata returned by the
// integration client. Callers are responsible for waiting until retries finish.
func RecordIntegrationResult(integration, result string) {
	integrationAttempts.WithLabelValues(integration, result).Inc()
}
