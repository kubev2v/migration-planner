package apiserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/api/server"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/service"
	"github.com/kubev2v/migration-planner/internal/store"
	oapimiddleware "github.com/oapi-codegen/nethttp-middleware"
	"github.com/sirupsen/logrus"
)

const (
	gracefulShutdownTimeout = 5 * time.Second
)

type Server struct {
	log      logrus.FieldLogger
	cfg      *config.Config
	store    store.Store
	listener net.Listener
}

// New returns a new instance of a migration-planner server.
func New(
	log logrus.FieldLogger,
	cfg *config.Config,
	store store.Store,
	listener net.Listener,
) *Server {
	return &Server{
		log:      log,
		cfg:      cfg,
		store:    store,
		listener: listener,
	}
}

func oapiErrorHandler(w http.ResponseWriter, message string, statusCode int) {
	http.Error(w, fmt.Sprintf("API Error: %s", message), statusCode)
}

func (s *Server) Run(ctx context.Context) error {
	s.log.Println("Initializing API server")
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
	router.Use(
		middleware.RequestID,
		middleware.Logger,
		middleware.Recoverer,
		oapimiddleware.OapiRequestValidatorWithOptions(swagger, &oapiOpts),
	)

	h := service.NewServiceHandler(s.store, s.log)
	server.HandlerFromMux(server.NewStrictHandler(h, nil), router)

	/*
		srv := tlsmiddleware.NewHTTPServerWithTLSContext(router, s.log, s.cfg.Service.Address)

		go func() {
			<-ctx.Done()
			s.log.Println("Shutdown signal received:", ctx.Err())
			ctxTimeout, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
			defer cancel()

			srv.SetKeepAlivesEnabled(false)
			_ = srv.Shutdown(ctxTimeout)
		}()

		s.log.Printf("Listening on %s...", s.listener.Addr().String())
		if err := srv.Serve(s.listener); err != nil && !errors.Is(err, net.ErrClosed) {
			return err
		}*/

	return nil
}
