package agent

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
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
	tlsConfig     *tls.Config
	logger        *zap.SugaredLogger
}

func NewServer(port int, configuration *config.Config, cert *x509.Certificate, certPrivateKey *rsa.PrivateKey) *Server {
	logger := zap.S().Named("server")

	s := &Server{
		port:          port,
		dataFolder:    configuration.DataDir,
		wwwFolder:     configuration.WwwDir,
		configuration: configuration,
		logger:        logger,
	}

	if cert != nil && certPrivateKey != nil {
		tlsConfig, err := getTlsConfig(cert, certPrivateKey)
		if err != nil {
			s.logger.Errorf("failed to create tls configuration: %s", err)
			return s
		}

		s.tlsConfig = tlsConfig
	}

	return s
}

func (s *Server) Start(statusUpdater *service.StatusUpdater) {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.Logger)

	RegisterFileServer(router, s.wwwFolder)
	RegisterApi(router, statusUpdater, s.configuration)

	s.restServer = &http.Server{Addr: fmt.Sprintf("0.0.0.0:%d", s.port), Handler: router}

	if s.tlsConfig != nil {
		s.logger.Infof("tls configured")
		s.restServer.TLSConfig = s.tlsConfig
		if err := s.restServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			s.logger.Fatalf("failed to start server with tls: %w", err)
		}
	}

	// Run the server without tls
	err := s.restServer.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		s.logger.Fatalf("failed to start server: %w", err)
	}
}

func (s *Server) Stop(stopCh chan any) {
	shutdownCtx, _ := context.WithTimeout(context.Background(), 10*time.Second) // nolint:govet
	doneCh := make(chan any)

	go func() {
		err := s.restServer.Shutdown(shutdownCtx)
		if err != nil {
			s.logger.Errorf("failed to graceful shutdown the server: %s", err)
		}
		close(doneCh)
	}()

	<-doneCh

	close(stopCh)
}

// getTLSConfig tries to create a tls configuration
func getTlsConfig(cert *x509.Certificate, privateKey *rsa.PrivateKey) (*tls.Config, error) {
	certPEM := new(bytes.Buffer)
	if err := pem.Encode(certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	}); err != nil {
		return nil, err
	}

	privKeyPEM := new(bytes.Buffer)
	if err := pem.Encode(privKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}); err != nil {
		return nil, err
	}

	serverCert, err := tls.X509KeyPair(certPEM.Bytes(), privKeyPEM.Bytes())
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{serverCert},
	}, nil
}
