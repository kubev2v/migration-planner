package apiserver

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/go-chi/chi"
	"github.com/kubev2v/migration-planner/pkg/metrics"
	"go.uber.org/zap"
)

type MetricServer struct {
	bindAddress string
	httpServer  *http.Server
	listener    net.Listener
}

func NewMetricServer(bindAddress string, listener net.Listener) *MetricServer {
	router := chi.NewRouter()

	prometheusMetricHandler := metrics.NewPrometheusMetricsHandler()
	router.Handle("/metrics", prometheusMetricHandler.Handler())

	s := &MetricServer{
		bindAddress: bindAddress,
		listener:    listener,
		httpServer: &http.Server{
			Addr:    bindAddress,
			Handler: router,
		},
	}

	return s
}

func (m *MetricServer) Run(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		ctxTimeout, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
		defer cancel()

		m.httpServer.SetKeepAlivesEnabled(false)
		_ = m.httpServer.Shutdown(ctxTimeout)
		zap.S().Named("metrics_server").Info("metrics server terminated")
	}()

	zap.S().Named("metrics_server").Infof("serving metrics: %s", m.bindAddress)
	if err := m.httpServer.Serve(m.listener); err != nil && !errors.Is(err, net.ErrClosed) {
		return err
	}
	return nil
}
