package apiserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

const oldSchemaErrorMessage = "The uploaded file is using an old schema version and cannot be parsed. Generate a new OVA file, import to your vSphere environment and then try to upload it again."

func oapiErrorHandler(w http.ResponseWriter, message string, statusCode int) {
	http.Error(w, fmt.Sprintf("API Error: %s", message), statusCode)
}

// detectOldSchemaMiddleware checks for old inventory schema format before OpenAPI validation.
// Old schema: inventory.{infra, vcenter, vms} - VMs at top level
// New schema: inventory.{clusters, vcenter, vcenter_id} - VMs inside clusters/vcenter
func detectOldSchemaMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimSuffix(r.URL.Path, "/")
		if r.Method != http.MethodPut || !strings.HasSuffix(path, "/inventory") {
			next.ServeHTTP(w, r)
			return
		}

		body, err := io.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusInternalServerError)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))

		var payload map[string]any
		if json.Unmarshal(body, &payload) == nil {
			if inv, ok := payload["inventory"].(map[string]any); ok {
				_, hasVms := inv["vms"]
				clusters := inv["clusters"]
				zap.S().Named("api_server").Debugw("detectOldSchemaMiddleware: checking schema",
					"hasVms", hasVms,
					"clusters", clusters,
					"clustersIsNil", clusters == nil,
				)
				if hasVms || clusters == nil {
					zap.S().Named("api_server").Infow("Rejected inventory upload with old schema format",
						"path", r.URL.Path,
						"method", r.Method,
					)
					http.Error(w, oldSchemaErrorMessage, http.StatusBadRequest)
					return
				}
			}
		}

		next.ServeHTTP(w, r)
	})
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
		detectOldSchemaMiddleware,
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
