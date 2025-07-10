package apiserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/config"
	handlers "github.com/kubev2v/migration-planner/internal/handlers/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/image"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/pkg/log"
	"github.com/kubev2v/migration-planner/pkg/metrics"
	oapimiddleware "github.com/oapi-codegen/nethttp-middleware"
	"go.uber.org/zap"
)

const (
	gracefulShutdownTimeout = 5 * time.Second
)

type Server struct {
	cfg      *config.Config
	store    store.Store
	listener net.Listener
}

// New returns a new instance of a migration-planner server.
func New(
	cfg *config.Config,
	store store.Store,
	listener net.Listener,
) *Server {
	return &Server{
		cfg:      cfg,
		store:    store,
		listener: listener,
	}
}

func oapiErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var statusCode int

	// Check if error implements HTTPError interface
	if httpErr, ok := err.(service.HTTPStatusCodeError); ok {
		statusCode = httpErr.HTTPStatusCode()
	} else {
		// Fallback: check for known error types and patterns
		statusCode = getStatusCodeFromError(err)
	}

	http.Error(w, fmt.Sprintf("API Error: %s", err.Error()), statusCode)
}

// getStatusCodeFromError provides fallback status code mapping for errors that don't implement HTTPError
func getStatusCodeFromError(err error) int {
	// Check error message patterns as last resort
	errMsg := strings.ToLower(err.Error())
	if strings.Contains(errMsg, "not found") {
		return http.StatusNotFound
	} else if strings.Contains(errMsg, "unauthorized") || strings.Contains(errMsg, "authentication") {
		return http.StatusUnauthorized
	} else if strings.Contains(errMsg, "forbidden") || strings.Contains(errMsg, "permission") {
		return http.StatusForbidden
	}

	return http.StatusBadRequest
}

// Middleware to inject ResponseWriter into context
func WithResponseWriter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add ResponseWriter to context
		ctx := context.WithValue(r.Context(), image.ResponseWriterKey, w)
		// Pass the modified context to the next handler
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) Run(ctx context.Context) error {
	zap.S().Named("api_server").Info("Initializing API server")
	swagger, err := api.GetSwagger()
	if err != nil {
		return fmt.Errorf("failed to load swagger spec: %w", err)
	}
	// Skip server name validation
	swagger.Servers = nil

	oapiOpts := oapimiddleware.Options{}

	authenticator, err := auth.NewAuthenticator(s.cfg.Service.Auth)
	if err != nil {
		return fmt.Errorf("failed to create authenticator: %w", err)
	}

	router := chi.NewRouter()

	metricMiddleware := metrics.NewMiddleware("api_server")
	metricMiddleware.MustRegisterDefault()

	// Common middleware for all routes
	router.Use(
		metricMiddleware.Handler,
		cors.Handler(cors.Options{
			AllowedOrigins:   []string{"https://console.stage.redhat.com", "https://stage.foo.redhat.com:1337"},
			AllowedMethods:   []string{"GET", "PUT", "POST", "DELETE", "HEAD", "OPTIONS"},
			AllowedHeaders:   []string{"*"},
			AllowCredentials: true,
			MaxAge:           300,
		}),
		middleware.RequestID,
		log.ConditionalLogger(s.cfg.Service.LogLevel, zap.L(), "router_api"),
		middleware.Recoverer,
		WithResponseWriter,
	)

	h := handlers.NewServiceHandler(service.NewSourceService(s.store), service.NewShareTokenService(s.store))
	strictHandler := server.NewStrictHandler(h, nil)
	wrapper := &server.ServerInterfaceWrapper{
		Handler:            strictHandler,
		HandlerMiddlewares: nil,
		ErrorHandlerFunc:   oapiErrorHandler,
	}

	// Route group for public endpoints (no authentication required)
	router.Group(func(r chi.Router) {
		r.Use(oapimiddleware.OapiRequestValidatorWithOptions(swagger, &oapiOpts))
		r.Get("/api/v1/shared/{token}", wrapper.GetSharedSource)
	})

	// Route group for authenticated endpoints
	router.Group(func(r chi.Router) {
		r.Use(
			authenticator.Authenticator,
			oapimiddleware.OapiRequestValidatorWithOptions(swagger, &oapiOpts),
		)
		server.HandlerFromMux(strictHandler, r)
	})
	srv := http.Server{Addr: s.cfg.Service.Address, Handler: router}

	go func() {
		<-ctx.Done()
		zap.S().Named("api_server").Infof("Shutdown signal received: %s", ctx.Err())
		ctxTimeout, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
		defer cancel()

		srv.SetKeepAlivesEnabled(false)
		_ = srv.Shutdown(ctxTimeout)
	}()

	zap.S().Named("api_server").Infof("Listening on %s...", s.listener.Addr().String())
	if err := srv.Serve(s.listener); err != nil && !errors.Is(err, net.ErrClosed) {
		return err
	}

	return nil
}
