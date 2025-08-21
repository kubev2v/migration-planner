package isoserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

const (
	gracefulShutdownTimeout = 5 * time.Second
	IsoEndpoint             = "/iso"
	HealthEndpoint          = "/healthz"
)

type IsoServer struct {
	address  string
	listener net.Listener
	isoPath  string
}

func New(
	listener net.Listener,
	address, isoPath string,
) *IsoServer {
	return &IsoServer{
		address:  address,
		listener: listener,
		isoPath:  isoPath,
	}
}

func (s *IsoServer) Run(ctx context.Context) error {
	zap.S().Named("iso_server").Info("Initializing iso server")

	router := chi.NewRouter()

	router.Use(
		middleware.Recoverer,
		middleware.Logger,
	)

	router.Get(HealthEndpoint, s.healthHandler)
	router.Get(IsoEndpoint, s.serveIsoHandler)
	router.Head(IsoEndpoint, s.isoHeadHandler)

	srv := http.Server{Addr: s.address, Handler: router}

	go func() {
		<-ctx.Done()
		zap.S().Named("iso_server").Infof("Shutdown signal received: %s", ctx.Err())
		ctxTimeout, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
		defer cancel()

		srv.SetKeepAlivesEnabled(false)
		_ = srv.Shutdown(ctxTimeout)
	}()

	zap.S().Named("iso_server").Infof("Listening on %s...", s.listener.Addr().String())
	zap.S().Named("iso_server").Infof("Serving ISO from: %s", s.isoPath)
	if err := srv.Serve(s.listener); err != nil && !errors.Is(err, net.ErrClosed) {
		return err
	}

	return nil
}

func (s *IsoServer) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (s *IsoServer) isoHeadHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := os.Stat(s.isoPath); err != nil {
		zap.S().Named("iso_server").Warnf("ISO HEAD check failed: ISO not available at %s", s.isoPath)
		http.Error(w, "ISO file not available", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
}

func (s *IsoServer) serveIsoHandler(w http.ResponseWriter, r *http.Request) {
	s.serveFile(w, r, s.isoPath)
}

func (s *IsoServer) serveFile(w http.ResponseWriter, r *http.Request, filePath string) {
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "File not found", http.StatusNotFound)
		} else {
			http.Error(w, "File access error", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filepath.Base(filePath)))

	http.ServeFile(w, r, filePath)
	zap.S().Named("iso_server").Infof("Served file: %s (%d bytes)", filePath, info.Size())
}
