package apiserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/config"
	handlers "github.com/kubev2v/migration-planner/internal/handlers/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/image"
	"github.com/kubev2v/migration-planner/internal/opa"
	"github.com/kubev2v/migration-planner/internal/rvtools"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/pkg/metrics"
	"github.com/kubev2v/migration-planner/pkg/middleware"
	oapimiddleware "github.com/oapi-codegen/nethttp-middleware"
	"go.uber.org/zap"
)

const (
	gracefulShutdownTimeout = 5 * time.Second
)

type Server struct {
	cfg          *config.Config
	store        store.Store
	listener     net.Listener
	opaValidator *opa.Validator
}

// New returns a new instance of a migration-planner server.
func New(
	cfg *config.Config,
	store store.Store,
	listener net.Listener,
	opaValidator *opa.Validator,
) *Server {
	return &Server{
		cfg:          cfg,
		store:        store,
		listener:     listener,
		opaValidator: opaValidator,
	}
}

func oapiErrorHandler(w http.ResponseWriter, message string, statusCode int) {
	http.Error(w, fmt.Sprintf("API Error: %s", message), statusCode)
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

	oapiOpts := oapimiddleware.Options{
		ErrorHandler: oapiErrorHandler,
	}

	authenticator, err := auth.NewAuthenticator(s.cfg.Service.Auth)
	if err != nil {
		return fmt.Errorf("failed to create authenticator: %w", err)
	}

	router := chi.NewRouter()

	metricMiddleware := metrics.NewMiddleware("api_server")
	metricMiddleware.MustRegisterDefault()

	router.Use(
		metricMiddleware.Handler,
		cors.Handler(cors.Options{
			AllowedOrigins:   []string{"https://console.stage.redhat.com", "https://stage.foo.redhat.com:1337"},
			AllowedMethods:   []string{"GET", "PUT", "POST", "DELETE", "HEAD", "OPTIONS"},
			AllowedHeaders:   []string{"*"},
			AllowCredentials: true,
			MaxAge:           300,
		}),
		authenticator.Authenticator,
		middleware.RequestID,
		middleware.Logger(),
		chiMiddleware.Recoverer,
		oapimiddleware.OapiRequestValidatorWithOptions(swagger, &oapiOpts),
		WithResponseWriter,
	)

	// Initialize pgx pool for River
	var riverClient *river.Client[pgx.Tx]
	// Parse config to safely handle special characters in credentials
	dsn := fmt.Sprintf("host=%s user=%s password=%s port=%s dbname=%s",
		s.cfg.Database.Hostname,
		s.cfg.Database.User,
		s.cfg.Database.Password,
		s.cfg.Database.Port,
		s.cfg.Database.Name,
	)

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("failed to parse pgx config: %w", err)
	}

	// Configure connection pool for River's needs (including LISTEN/NOTIFY)
	cfg.MaxConns = 20 // Maximum connections for job processing + LISTEN
	cfg.MinConns = 5  // Keep connections warm for immediate job pickup
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 30 * time.Minute

	dbPool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to create pgx pool: %w", err)
	}
	defer dbPool.Close()

	// Initialize River workers
	workers := river.NewWorkers()
	river.AddWorker(workers, rvtools.NewRVToolsWorker(s.store, s.opaValidator))

	// Create River client with job retention policies
	riverClient, err = river.NewClient(riverpgxv5.New(dbPool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 4}, // Limit concurrent RVTools processing
		},
		Workers: workers,

		// Ultra-fast polling for immediate job pickup
		FetchCooldown:     50 * time.Millisecond,  // Check every 50ms when actively processing
		FetchPollInterval: 100 * time.Millisecond, // Check every 100ms when idle

		// Job retention policies to prevent database bloat
		CancelledJobRetentionPeriod: 24 * time.Hour,     // Keep cancelled jobs for 1 day
		CompletedJobRetentionPeriod: 24 * time.Hour,     // Keep completed jobs for 1 day
		DiscardedJobRetentionPeriod: 7 * 24 * time.Hour, // Keep failed jobs for 7 days (debugging)
	})
	if err != nil {
		return fmt.Errorf("failed to create river client: %w", err)
	}

	// Start River
	if err := riverClient.Start(ctx); err != nil {
		return fmt.Errorf("failed to start river: %w", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := riverClient.Stop(stopCtx); err != nil {
			zap.S().Named("api_server").Warnw("failed to stop river client", "error", err)
		}
	}()

	zap.S().Named("api_server").Info("River job queue initialized")

	assessmentService := service.NewAssessmentService(s.store, s.opaValidator, riverClient)

	h := handlers.NewServiceHandler(
		service.NewSourceService(s.store, s.opaValidator),
		assessmentService,
	)
	server.HandlerFromMux(server.NewStrictHandler(h, nil), router)
	srv := http.Server{Addr: s.cfg.Service.Address, Handler: router}

	go func() {
		<-ctx.Done()
		zap.S().Named("api_server").Infof("Shutdown signal received: %s", ctx.Err())
		ctxTimeout, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
		defer cancel()

		srv.SetKeepAlivesEnabled(false)
		_ = srv.Shutdown(ctxTimeout)
		zap.S().Named("api_server").Info("api server terminated")
	}()

	zap.S().Named("api_server").Infof("Listening on %s...", s.listener.Addr().String())
	if err := srv.Serve(s.listener); err != nil && !errors.Is(err, net.ErrClosed) {
		return err
	}

	return nil
}
