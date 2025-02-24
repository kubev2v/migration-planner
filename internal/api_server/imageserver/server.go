package imageserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	api "github.com/kubev2v/migration-planner/api/v1alpha1/image"
	server "github.com/kubev2v/migration-planner/internal/api/server/image"
	apiserver "github.com/kubev2v/migration-planner/internal/api_server"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/events"
	service "github.com/kubev2v/migration-planner/internal/service/image"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/leosunmo/zapchi"
	oapimiddleware "github.com/oapi-codegen/nethttp-middleware"
	chiprometheus "github.com/toshi0607/chi-prometheus"
	"go.uber.org/zap"
)

const (
	gracefulShutdownTimeout = 5 * time.Second
)

type ImageServer struct {
	svcCfg   *config.SvcConfig
	store    store.Store
	listener net.Listener
	evWriter *events.EventProducer
}

// New returns a new instance of a migration-planner server.
func New(
	svcCfg *config.SvcConfig,
	store store.Store,
	ew *events.EventProducer,
	listener net.Listener,
) *ImageServer {
	return &ImageServer{
		svcCfg:   svcCfg,
		store:    store,
		evWriter: ew,
		listener: listener,
	}
}

func oapiErrorHandler(w http.ResponseWriter, message string, statusCode int) {
	http.Error(w, fmt.Sprintf("API Error: %s", message), statusCode)
}

func (s *ImageServer) Run(ctx context.Context) error {
	zap.S().Named("image_server").Info("Initializing Image-side API server")
	swagger, err := api.GetSwagger()
	if err != nil {
		return fmt.Errorf("failed loading swagger spec: %w", err)
	}
	// Skip server name validation
	swagger.Servers = nil

	oapiOpts := oapimiddleware.Options{
		ErrorHandler: oapiErrorHandler,
	}

	router := chi.NewRouter()

	metricMiddleware := chiprometheus.New("image_server")
	metricMiddleware.MustRegisterDefault()

	router.Use(
		metricMiddleware.Handler,
		middleware.RequestID,
		zapchi.Logger(zap.S(), "router_image"),
		middleware.Recoverer,
		oapimiddleware.OapiRequestValidatorWithOptions(swagger, &oapiOpts),
		apiserver.WithResponseWriter,
	)

	h := service.NewImageHandler(s.store, s.evWriter)
	server.HandlerFromMux(server.NewStrictHandler(h, nil), router)
	srv := http.Server{Addr: s.svcCfg.Address, Handler: router}

	go func() {
		<-ctx.Done()
		zap.S().Named("image_server").Infof("Shutdown signal received: %s", ctx.Err())
		ctxTimeout, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
		defer cancel()

		srv.SetKeepAlivesEnabled(false)
		_ = srv.Shutdown(ctxTimeout)
	}()

	zap.S().Named("image_server").Infof("Listening on %s...", s.listener.Addr().String())
	if err := srv.Serve(s.listener); err != nil && !errors.Is(err, net.ErrClosed) {
		return err
	}

	return nil
}
