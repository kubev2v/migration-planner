package apiserver

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"

	"github.com/go-chi/chi"
	"github.com/kubev2v/migration-planner/pkg/metrics"
	"go.uber.org/zap"
)

type MetricServer struct {
	bindAddress string
	httpServer  *http.Server
	listener    net.Listener
	wg          *sync.WaitGroup
}

func NewMetricServer(bindAddress string, listener net.Listener, wg *sync.WaitGroup) *MetricServer {
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
		wg: wg,
	}

	return s
}

func (m *MetricServer) Run(ctx context.Context) error {
	m.wg.Add(1)
	go func() {
		<-ctx.Done()
		ctxTimeout, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
		defer cancel()

		m.httpServer.SetKeepAlivesEnabled(false)
		_ = m.httpServer.Shutdown(ctxTimeout)
		zap.S().Named("metrics_server").Info("metrics server terminated")
		m.wg.Done()
	}()

	zap.S().Named("metrics_server").Infof("serving metrics: %s", m.bindAddress)
	if err := m.httpServer.Serve(m.listener); err != nil && !errors.Is(err, net.ErrClosed) {
		return err
	}
	return nil
}
