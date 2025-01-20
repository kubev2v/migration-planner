package agent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/kubev2v/migration-planner/internal/agent/config"
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
	port          int
	dataFolder    string
	wwwFolder     string
	configuration *config.Config
	restServer    *http.Server
}

func NewServer(port int, configuration *config.Config) *Server {
	return &Server{
		port:          port,
		dataFolder:    configuration.DataDir,
		wwwFolder:     configuration.WwwDir,
		configuration: configuration,
	}
}

func (s *Server) Start(statusUpdater *service.StatusUpdater) {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.Logger)

	RegisterFileServer(router, s.wwwFolder)
	RegisterApi(router, statusUpdater, s.configuration)

	s.restServer = &http.Server{Addr: fmt.Sprintf("0.0.0.0:%d", s.port), Handler: router}

	if tlsConfig, err := s.getTLSConfig(); err == nil {
		zap.S().Named("server").Infof("setup tls configuration")
		s.restServer.TLSConfig = tlsConfig

		// Run the server
		err := s.restServer.ListenAndServeTLS("", "")
		if err != nil && err != http.ErrServerClosed {
			zap.S().Named("server").Fatalf("failed to start server: %w", err)
		}

		return
	}

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

// getTLSConfig tries to create a tls configuration
// It looks for certificates in the config folder
func (s *Server) getTLSConfig() (*tls.Config, error) {
	cert, err := os.ReadFile(fmt.Sprintf("%s/agent_ui.crt", s.configuration.ConfigDir))
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate file: %w", err)
	}

	key, err := os.ReadFile(fmt.Sprintf("%s/agent_ui.key", s.configuration.ConfigDir))
	if err != nil {
		return nil, fmt.Errorf("failed to read agent key file: %w", err)
	}

	caCert, err := os.ReadFile(fmt.Sprintf("%s/agent_ui_ca.crt", s.configuration.ConfigDir))
	if err != nil {
		return nil, fmt.Errorf("failed to read ca certificate file: %w", err)
	}

	serverCert, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return nil, err
	}

	certpool := x509.NewCertPool()
	if ok := certpool.AppendCertsFromPEM(caCert); !ok {
		return nil, errors.New("failed to append ca certificate to capool")
	}

	return &tls.Config{
		RootCAs:      certpool,
		Certificates: []tls.Certificate{serverCert},
	}, nil
}
