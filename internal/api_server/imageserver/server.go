package imageserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/kubev2v/migration-planner/internal/service"

	"github.com/kubev2v/migration-planner/pkg/log"
	"github.com/kubev2v/migration-planner/pkg/metrics"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	api "github.com/kubev2v/migration-planner/api/v1alpha1/image"
	server "github.com/kubev2v/migration-planner/internal/api/server/image"
	"github.com/kubev2v/migration-planner/internal/config"
	handlers "github.com/kubev2v/migration-planner/internal/handlers/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/store"
	oapimiddleware "github.com/oapi-codegen/nethttp-middleware"
	"go.uber.org/zap"
)

const (
	gracefulShutdownTimeout = 5 * time.Second
)

type ImageServer struct {
	cfg      *config.Config
	store    store.Store
	listener net.Listener
}

// New returns a new instance of a migration-planner server.
func New(
	cfg *config.Config,
	store store.Store,
	listener net.Listener,
) *ImageServer {
	return &ImageServer{
		cfg:      cfg,
		store:    store,
		listener: listener,
	}
}

func oapiErrorHandler(w http.ResponseWriter, message string, statusCode int) {
	http.Error(w, fmt.Sprintf("API Error: %s", message), statusCode)
}

// Middleware to inject request into context
func withRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), handlers.RequestKey, r)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Middleware to inject ResponseWriter into context
func withResponseWriter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add ResponseWriter to context
		ctx := context.WithValue(r.Context(), handlers.ResponseWriterKey, w)
		// Pass the modified context to the next handler
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *ImageServer) Run(ctx context.Context) error {
	zap.S().Named("image_server").Info("Initializing Image-side API server")
	swagger, err := api.GetSwagger()
	if err != nil {
		return fmt.Errorf("failed to load swagger spec: %w", err)
	}
	// Skip server name validation
	swagger.Servers = nil

	oapiOpts := oapimiddleware.Options{
		ErrorHandler: oapiErrorHandler,
	}

	router := chi.NewRouter()

	metricMiddleware := metrics.NewMiddleware("image_server")
	metricMiddleware.MustRegisterDefault()

	router.Use(
		metricMiddleware.Handler,
		middleware.RequestID,
		log.ConditionalLogger(s.cfg.Service.LogLevel, zap.L(), "image_server"),
		middleware.Recoverer,
		oapimiddleware.OapiRequestValidatorWithOptions(swagger, &oapiOpts),
		withResponseWriter,
		withRequest,
		cors.Handler(cors.Options{
			AllowedOrigins: []string{"https://console.stage.redhat.com", "https://stage.foo.redhat.com:1337"},
			AllowedMethods: []string{"GET", "OPTIONS"},
			AllowedHeaders: []string{"*"},
			MaxAge:         300,
		}),
	)

	if err := os.MkdirAll(s.cfg.Service.TempImagesDir, 0700); err != nil {
		return fmt.Errorf("create temp images folder %q: %w", s.cfg.Service.TempImagesDir, err)
	}

	h := handlers.NewImageHandler(service.NewImageSvc(s.store, s.cfg.Service.TempImagesDir, s.cfg.Service.TempImagesDirLimit))
	server.HandlerFromMux(server.NewStrictHandler(h, nil), router)
	srv := http.Server{Addr: s.cfg.Service.Address, Handler: router}

	go func() {
		<-ctx.Done()
		zap.S().Named("image_server").Infof("Shutdown signal received: %s", ctx.Err())
		ctxTimeout, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
		defer cancel()

		srv.SetKeepAlivesEnabled(false)
		_ = srv.Shutdown(ctxTimeout)
		zap.S().Named("image_server").Info("image server terminated")
	}()

	zap.S().Named("image_server").Infof("Listening on %s...", s.listener.Addr().String())
	if err := srv.Serve(s.listener); err != nil && !errors.Is(err, net.ErrClosed) {
		return err
	}

	return nil
}
