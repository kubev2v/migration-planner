package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	apiserver "github.com/kubev2v/migration-planner/internal/api_server"
	"github.com/kubev2v/migration-planner/internal/api_server/agentserver"
	"github.com/kubev2v/migration-planner/internal/api_server/imageserver"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/opa"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/pkg/iso"
	"github.com/kubev2v/migration-planner/pkg/log"
	"github.com/kubev2v/migration-planner/pkg/metrics"
	"github.com/kubev2v/migration-planner/pkg/migrations"
	"github.com/kubev2v/migration-planner/pkg/version"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

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

		// Initialize database with basic example report
		if v, b := os.LookupEnv("NO_SEED"); !b || v == "false" {
			if err := store.Seed(); err != nil {
				zap.S().Fatalw("seeding database with default report", "error", err)
			}
		}

		// Initialize ISOs
		zap.S().Info("Initializing RHCOS ISO")
		if err := initializeIso(context.TODO(), cfg); err != nil {
			zap.S().Fatalw("failed to initilized iso", "error", err)
		}
		zap.S().Info("RHCOS ISO initialized")

		// Initialize OPA validator for policy validation
		zap.S().Info("initializing OPA validator...")
		opaValidator, err := opa.NewValidatorFromDir(cfg.Service.OpaPoliciesFolder)
		if err != nil {
			zap.S().Warnf("Failed to initialize OPA validator: %v - validation will be disabled", err)
		}

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGQUIT)

		go func() {
			defer cancel()
			listener, err := newListener(cfg.Service.Address)
			if err != nil {
				zap.S().Fatalw("creating listener", "error", err)
			}

			server := apiserver.New(cfg, store, listener, opaValidator)
			if err := server.Run(ctx); err != nil {
				zap.S().Fatalw("Error running server", "error", err)
			}
		}()

		// register metrics
		metrics.RegisterMetrics(store)

		go func() {
			defer cancel()
			listener, err := newListener(cfg.Service.AgentEndpointAddress)
			if err != nil {
				zap.S().Fatalw("creating listener", "error", err)
			}

			agentserver := agentserver.New(cfg, store, listener)
			if err := agentserver.Run(ctx); err != nil {
				zap.S().Fatalw("Error running server", "error", err)
			}
		}()

		go func() {
			defer cancel()
			listener, err := newListener(cfg.Service.ImageEndpointAddress)
			if err != nil {
				zap.S().Fatalw("creating listener", "error", err)
			}

			imageserver := imageserver.New(cfg, store, listener)
			if err := imageserver.Run(ctx); err != nil {
				zap.S().Fatalw("Error running server", "error", err)
			}
		}()

		go func() {
			defer cancel()
			listener, err := newListener("0.0.0.0:8080")
			if err != nil {
				zap.S().Named("metrics_server").Fatalw("creating listener", "error", err)
			}
			metricsServer := apiserver.NewMetricServer("0.0.0.0:8080", listener)
			if err := metricsServer.Run(ctx); err != nil {
				zap.S().Named("metrics_server").Fatalw("failed to run metrics server", "error", err)
			}
		}()

		<-ctx.Done()

		return nil
	},
}

func newListener(address string) (net.Listener, error) {
	if address == "" {
		address = "localhost:0"
	}
	return net.Listen("tcp", address)
}

func initializeIso(ctx context.Context, cfg *config.Config) error {
	// Check if ISO already exists:
	isoPath := cfg.Service.IsoPath
	if _, err := os.Stat(isoPath); err == nil {
		return nil
	}

	out, err := os.Create(isoPath)
	if err != nil {
		return err
	}
	defer out.Close()

	md := iso.NewDownloaderManager()

	isoServerUrl := fmt.Sprintf("%s:%s", cfg.Service.IsoServerBaseURL, cfg.Service.IsoServerPort)
	isoServerDownloader, err := iso.NewIsoServerDownloader(
		isoServerUrl,
		cfg.Service.RhcosImageSha256,
	)

	if err == nil {
		if err := isoServerDownloader.HealthCheck(ctx); err == nil {
			md.Register(isoServerDownloader)
			zap.S().Infow("Registered ISO server downloader", "url", isoServerUrl)
		}
	}

	// Fallback: Official RHCOS HTTP downloader
	md.Register(iso.NewHttpDownloader(cfg.Service.RhcosImageName, cfg.Service.RhcosImageSha256))

	if err := md.Download(ctx, out); err != nil {
		// Remove the file from disk to allow the planner to retry the image download after restart.
		_ = os.Remove(isoPath)
		return err
	}

	return nil
}
