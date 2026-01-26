package apiserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/kubev2v/migration-planner/pkg/opa"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/auth"
	"github.com/kubev2v/migration-planner/internal/client"
	"github.com/kubev2v/migration-planner/internal/config"
	handlers "github.com/kubev2v/migration-planner/internal/handlers/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/image"
	"github.com/kubev2v/migration-planner/internal/rvtools/jobs"
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
	jobsClient   *jobs.Client
}

// New returns a new instance of a migration-planner server.
func New(
	cfg *config.Config,
	store store.Store,
	listener net.Listener,
	opaValidator *opa.Validator,
	jobsClient *jobs.Client,
) *Server {
	return &Server{
		cfg:          cfg,
		store:        store,
		listener:     listener,
		opaValidator: opaValidator,
		jobsClient:   jobsClient,
	}
}

func oapiErrorHandler(w http.ResponseWriter, message string, statusCode int) {
	// Check if this is an old schema validation error
	// Only catch known old schema issues, not all UpdateInventory validation errors
	message = strings.TrimSpace(message)
	lower := strings.ToLower(message)

	// Check for UpdateInventory schema errors related to old schema format
	hasUpdateInventory := strings.Contains(lower, "updateinventory") ||
		strings.Contains(lower, "#/components/schemas/updateinventory")

	// Only treat as old schema if it's a known old schema issue:
	// - Missing vms property: Error at "/inventory/vcenter/vms": property "vms" is missing
	// - Clusters nullable issue: Error at "/inventory/clusters": Value is not nullable
	// - Other inventory structure mismatches
	isOldSchema := hasUpdateInventory &&
		(strings.Contains(lower, `"/inventory/vcenter/vms"`) ||
			strings.Contains(lower, `property "vms" is missing`) ||
			strings.Contains(lower, `"/inventory/clusters"`) ||
			strings.Contains(lower, `value is not nullable`))

	if isOldSchema {
		http.Error(w, "The uploaded file is using an old schema version and cannot be parsed. Generate a new OVA file, import to your vSphere environment and then try to upload it again.", statusCode)
		return
	}
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

	// Initialize sizer client
	sizerTimeout, err := time.ParseDuration(s.cfg.Service.Sizer.Timeout)
	if err != nil {
		zap.S().Named("api_server").Warnf("Invalid sizer timeout, using default 60s: %v", err)
		sizerTimeout = 60 * time.Second
	}
	sizerClient := client.NewSizerClient(s.cfg.Service.Sizer.ServiceURL, sizerTimeout)

	h := handlers.NewServiceHandler(
		service.NewSourceService(s.store, s.opaValidator),
		service.NewAssessmentService(s.store, s.opaValidator),
		service.NewJobService(s.store, s.jobsClient.RiverClient),
		service.NewSizerService(sizerClient, s.store),
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
