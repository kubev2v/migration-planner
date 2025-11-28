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
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kubev2v/migration-planner/internal/api_server/agentserver"
	"github.com/kubev2v/migration-planner/internal/api_server/imageserver"
	"github.com/kubev2v/migration-planner/internal/rvtools/jobs"
	"github.com/kubev2v/migration-planner/internal/service"
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

		pgxDSN := fmt.Sprintf("host=%s user=%s password=%s port=%s dbname=%s",
			cfg.Database.Hostname,
			cfg.Database.User,
			cfg.Database.Password,
			cfg.Database.Port,
			cfg.Database.Name,
		)
		pgxCfg, err := pgxpool.ParseConfig(pgxDSN)
		if err != nil {
			zap.S().Fatalw("parsing pgx config", "error", err)
		}

		// Configure pool for River's LISTEN/NOTIFY (instant job notifications)
		pgxCfg.MaxConns = 20
		pgxCfg.MinConns = 2
		pgxCfg.MaxConnLifetime = time.Hour
		pgxCfg.MaxConnIdleTime = 30 * time.Minute

		pgxPool, err := pgxpool.NewWithConfig(context.Background(), pgxCfg)
		if err != nil {
			zap.S().Fatalw("creating pgx pool", "error", err)
		}
		defer pgxPool.Close()

		if err := migrations.MigrateStore(db, cfg.Service.MigrationFolder, pgxPool); err != nil {
			zap.S().Fatalw("running migrations", "error", err)
		}

		if err := ensureIsoExist(cfg.Service.IsoPath); err != nil {
			zap.S().Fatalw("validate iso", "error", err)
			return err
		}

		zap.S().Info("Initializing OPA validator...")
		opaValidator, err := opa.NewValidatorFromDir(cfg.Service.OpaPoliciesFolder)
		if err != nil {
			zap.S().Warnf("Failed to initialize OPA validator: %v - validation will be disabled", err)
		}

		zap.S().Info("Initializing River Queue...")
		jobClient, err := jobs.NewClient(context.Background(), pgxPool, opaValidator)
		if err != nil {
			zap.S().Fatalw("creating River client", "error", err)
		}
		jobService := service.NewJobService(jobClient)

		// Start River workers (non-blocking in v0.28+)
		if err := jobClient.Start(context.Background()); err != nil {
			zap.S().Fatalw("starting River client", "error", err)
		}
		zap.S().Info("River Queue started")

		// Ensure graceful shutdown of River
		defer func() {
			zap.S().Info("Stopping River Queue...")
			stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer stopCancel()
			if err := jobClient.Stop(stopCtx); err != nil {
				zap.S().Warnw("failed to stop River client gracefully", "error", err)
			}
			zap.S().Info("River Queue stopped")
		}()

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGQUIT)
		var wg sync.WaitGroup

		metrics.RegisterMetrics(store)

		runServer(ctx, &wg, cancel, cfg.Service.Address, "api_server", func(l net.Listener) Server {
			return apiserver.New(cfg, store, l, opaValidator, jobService)
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
