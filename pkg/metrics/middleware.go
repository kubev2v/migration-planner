package metrics

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	bucketsConfig = []float64{300, 500, 1000, 5000}
)

const (
	// EnvChiPrometheusLatencyBuckets represents an environment variable, which is formatted like "100,200,300,400" as string
	EnvChiPrometheusLatencyBuckets = "CHI_PROMETHEUS_LATENCY_BUCKETS"
	RequestsCollectorName          = "chi_requests_total"
	LatencyCollectorName           = "chi_request_duration_milliseconds"
)

// Middleware is a handler that exposes prometheus metrics for the number of requests,
// the latency, and the response size partitioned by status code, method, and HTTP path.
type Middleware struct {
	requests *prometheus.CounterVec
	latency  *prometheus.HistogramVec
}

func setBucket() {
	var buckets []float64
	conf, ok := os.LookupEnv(EnvChiPrometheusLatencyBuckets)
	if ok {
		for _, v := range strings.Split(conf, ",") {
			f64v, err := strconv.ParseFloat(v, 64)
			if err != nil {
				panic(err)
			}
			buckets = append(buckets, f64v)
		}
		bucketsConfig = buckets
	}
}

// New returns a new prometheus middleware for the provided service name.
func NewMiddleware(name string) *Middleware {
	setBucket()

	var m Middleware
	m.requests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        RequestsCollectorName,
			Help:        "Number of HTTP requests partitioned by status code, method and HTTP path.",
			ConstLabels: prometheus.Labels{"service": name},
		}, []string{"code", "method", "path"})

	m.latency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        LatencyCollectorName,
		Help:        "Time spent on the request partitioned by status code, method and HTTP path.",
		ConstLabels: prometheus.Labels{"service": name},
		Buckets:     bucketsConfig,
	}, []string{"code", "method", "path"})

	return &m
}

// Handler returns a handler for the middleware pattern.
func (m Middleware) Handler(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		if rctx := chi.RouteContext(r.Context()); rctx != nil {
			rp := rctx.RoutePattern()
			since := float64(time.Since(start).Milliseconds())
			m.requests.WithLabelValues(strconv.Itoa(ww.Status()), r.Method, rp).Inc()
			m.latency.WithLabelValues(strconv.Itoa(ww.Status()), r.Method, rp).Observe(since)
		}
	}
	return http.HandlerFunc(fn)
}

// Collectors returns collector for your own collector registry.
func (m Middleware) Collectors() []prometheus.Collector {
	return []prometheus.Collector{m.requests, m.latency}
}

// MustRegisterDefault registers collectors to DefaultRegisterer. If you don't initialize the collector registry,
// this method should be called before calling promhttp.Handler().
func (m Middleware) MustRegisterDefault() {
	if m.requests == nil || m.latency == nil {
		panic("collectors must be set")
	}
	prometheus.MustRegister(m.requests)
	prometheus.MustRegister(m.latency)
}
