package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/kubev2v/migration-planner/internal/api_server/agentserver"
	"github.com/kubev2v/migration-planner/internal/api_server/imageserver"
	"github.com/kubev2v/migration-planner/pkg/metrics"

	apiserver "github.com/kubev2v/migration-planner/internal/api_server"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/opa"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/pkg/log"
	"github.com/kubev2v/migration-planner/pkg/migrations"
	"github.com/kubev2v/migration-planner/pkg/version"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Server interface {
	Run(ctx context.Context) error
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the planner api",
	RunE: func(cmd *cobra.Command, args []string) error {
		defer zap.S().Info("API service stopped")

		cfg, err := config.New()
		if err != nil {
			zap.S().Fatalw("reading configuration", "error", err)
		}

		logLvl, err := zap.ParseAtomicLevel(cfg.Service.LogLevel)
		if err != nil {
			logLvl = zap.NewAtomicLevelAt(zapcore.InfoLevel)
		}

		logger := log.InitLog(logLvl)
		defer func() { _ = logger.Sync() }()

		undo := zap.ReplaceGlobals(logger)
		defer undo()

		zap.S().Info("Starting API service...")
		zap.S().Infow("Build from git", "commit", version.Get().GitCommit)
		zap.S().Info("Initializing data store")
		db, err := store.InitDB(cfg)
		if err != nil {
			zap.S().Fatalw("initializing data store", "error", err)
		}

		store := store.NewStore(db)
		defer store.Close()

		if err := migrations.MigrateStore(db, cfg.Service.MigrationFolder); err != nil {
			zap.S().Fatalw("running initial migration", "error", err)
		}

		// The migration planner API expects the RHCOS ISO to be on disk
		if err := ensureIsoExist(cfg.Service.IsoPath); err != nil {
			zap.S().Fatalw("validate iso", "error", err)
			return err
		}

		// Initialize OPA validator for policy validation
		zap.S().Info("initializing OPA validator...")
		opaValidator, err := opa.NewValidatorFromDir(cfg.Service.OpaPoliciesFolder)
		if err != nil {
			zap.S().Warnf("Failed to initialize OPA validator: %v - validation will be disabled", err)
		}

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGQUIT)
		var wg sync.WaitGroup // Responsible for keeping the main thread waiting for all goroutines to shut down gracefully
		// register metrics
		metrics.RegisterMetrics(store)

		runServer(ctx, &wg, cancel, cfg.Service.Address, "api_server", func(l net.Listener) Server {
			return apiserver.New(cfg, store, l, opaValidator)
		})

		runServer(ctx, &wg, cancel, cfg.Service.AgentEndpointAddress, "agent_server", func(l net.Listener) Server {
			return agentserver.New(cfg, store, l)
		})

		runServer(ctx, &wg, cancel, cfg.Service.ImageEndpointAddress, "image_server", func(l net.Listener) Server {
			return imageserver.New(cfg, store, l)
		})

		runServer(ctx, &wg, cancel, "0.0.0.0:8080", "metrics_server", func(l net.Listener) Server {
			return apiserver.NewMetricServer("0.0.0.0:8080", l)
		})

		<-ctx.Done()
		wg.Wait()
		zap.S().Info("Service stopped gracefully")

		return nil
	},
}

func newListener(address string) (net.Listener, error) {
	if address == "" {
		address = "localhost:0"
	}
	return net.Listen("tcp", address)
}

func ensureIsoExist(path string) error {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("RHCOS ISO not found at path: %s", path)
		} else if os.IsPermission(err) {
			return fmt.Errorf("permission denied for file: %s", path)
		}
		return err
	}
	return nil
}

func runServer(ctx context.Context, wg *sync.WaitGroup, cancel context.CancelFunc,
	address string, loggerName string, serverFactory func(net.Listener) Server) {

	wg.Add(1)

	go func() {
		defer func() {
			cancel()
			wg.Done()
		}()
		listener, err := newListener(address)
		if err != nil {
			zap.S().Named(loggerName).Errorw("creating listener", "error", err)
			return
		}

		server := serverFactory(listener)
		if err := server.Run(ctx); err != nil {
			zap.S().Named(loggerName).Errorw("Error running server", "error", err)
		}
	}()
}
