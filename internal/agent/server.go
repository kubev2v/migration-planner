package agent

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/cors"
	"github.com/kubev2v/migration-planner/pkg/log"
)

const (
	agentPort = 3333
)

func StartServer(log *log.PrefixLogger, config *Config) {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.Logger)
	router.Use(cors.Handler(cors.Options{
		AllowedMethods:   []string{"GET", "PUT", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	RegisterFileServer(router, log, config.WwwDir)
	RegisterApi(router, log, config.DataDir)

	server := &http.Server{Addr: fmt.Sprintf("0.0.0.0:%d", agentPort), Handler: router}
	serverCtx, serverStopCtx := context.WithCancel(context.Background())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-sig
		shutdownCtx, _ := context.WithTimeout(serverCtx, 30*time.Second) // nolint:govet

		go func() {
			<-shutdownCtx.Done()
			if shutdownCtx.Err() == context.DeadlineExceeded {
				log.Fatal("graceful shutdown timed out.. forcing exit.")
			}
		}()

		// Trigger graceful shutdown
		err := server.Shutdown(shutdownCtx)
		if err != nil {
			log.Fatal(err)
		}
		serverStopCtx()
	}()

	go func() {
		// Run the server
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Fatal(err.Error())
		}

		// Wait for server context to be stopped
		<-serverCtx.Done()
	}()
}
