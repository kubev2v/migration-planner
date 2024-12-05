package agent

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/kubev2v/migration-planner/internal/agent/service"
	"go.uber.org/zap"
)

/*
Server serves 3 endpoints:
- /login serves the credentials login form
- /api/v1/credentials called by the agent ui to pass the credentials entered by the user
- /api/v1/status return the status of the agent.
*/
type Server struct {
	port       int
	dataFolder string
	wwwFolder  string
	restServer *http.Server
}

func NewServer(port int, dataFolder, wwwFolder string) *Server {
	return &Server{
		port:       port,
		dataFolder: dataFolder,
		wwwFolder:  wwwFolder,
	}
}

func (s *Server) Start(statusUpdater *service.StatusUpdater) {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.Logger)

	RegisterFileServer(router, s.wwwFolder)
	RegisterApi(router, statusUpdater, s.dataFolder)

	s.restServer = &http.Server{Addr: fmt.Sprintf("0.0.0.0:%d", s.port), Handler: router}

	// Run the server
	err := s.restServer.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		zap.S().Named("server").Fatalf("failed to start server: %w", err)
	}
}

func (s *Server) Stop(stopCh chan any) {
	shutdownCtx, _ := context.WithTimeout(context.Background(), 10*time.Second) // nolint:govet
	doneCh := make(chan any)

	go func() {
		err := s.restServer.Shutdown(shutdownCtx)
		if err != nil {
			zap.S().Named("server").Errorf("failed to graceful shutdown the server: %s", err)
		}
		close(doneCh)
	}()

	<-doneCh

	close(stopCh)
}
