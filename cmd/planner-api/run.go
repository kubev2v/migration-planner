package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	apiserver "github.com/kubev2v/migration-planner/internal/api_server"
	"github.com/kubev2v/migration-planner/internal/api_server/agentserver"
	"github.com/kubev2v/migration-planner/internal/api_server/imageserver"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/opa"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/util"
	"github.com/kubev2v/migration-planner/pkg/iso"
	"github.com/kubev2v/migration-planner/pkg/log"
	"github.com/kubev2v/migration-planner/pkg/metrics"
	"github.com/kubev2v/migration-planner/pkg/migrations"
	"github.com/kubev2v/migration-planner/pkg/version"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func discoverPoliciesDirectory() string {
	candidates := []string{
		"./policies",    // Local development (make setup-opa-policies)
		"/app/policies", // Container deployment (downloaded during build)
	}

	for _, candidate := range candidates {
		if isPoliciesDirectory(candidate) {
			if absPath, err := filepath.Abs(candidate); err == nil {
				return absPath
			}
			return candidate
		}
	}

	return ""
}

func isPoliciesDirectory(dir string) bool {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return false
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.rego"))
	if err != nil {
		return false
	}

	return len(files) > 0
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

		// Initialize OPA manager for in-process validation
		zap.S().Info("Initializing OPA manager")
		policiesDir := discoverPoliciesDirectory()
		if policiesDir == "" {
			zap.S().Warn("No policies directory found")
			zap.S().Info("For local development, run: make setup-opa-policies")
			zap.S().Info("In containers, policies are available at /app/policies")
			policiesDir = "./policies"
		}

		opaManager := opa.NewManager(policiesDir)
		if err := opaManager.Initialize(); err != nil {
			zap.S().Warnf("Failed to initialize OPA manager: %v", err)
		} else {
			zap.S().Info("OPA manager initialized successfully")
		}

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

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGQUIT)
		go func() {
			defer cancel()
			listener, err := newListener(cfg.Service.Address)
			if err != nil {
				zap.S().Fatalw("creating listener", "error", err)
			}

			server := apiserver.New(cfg, store, listener, opaManager)
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

		// Handle OPA manager shutdown in a separate goroutine
		go func() {
			<-ctx.Done()
			// Shutdown OPA manager if it was initialized
			if opaManager != nil {
				zap.S().Info("Shutting down OPA manager")
				opaManager.Shutdown()
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
	isoPath := util.GetEnv("MIGRATION_PLANNER_ISO_PATH", "rhcos-live-iso.x86_64.iso")
	if _, err := os.Stat(isoPath); err == nil {
		return nil
	}

	out, err := os.Create(isoPath)
	if err != nil {
		return err
	}
	defer out.Close()

	md := iso.NewDownloaderManager()

	minio, err := iso.NewMinioDownloader(
		iso.WithEndpoint(cfg.Service.S3.Endpoint),
		iso.WithBucket(cfg.Service.S3.Bucket),
		iso.WithAccessKey(cfg.Service.S3.AccessKey),
		iso.WithSecretKey(cfg.Service.S3.SecretKey),
		iso.WithImageName(cfg.Service.S3.IsoFileName),
	)
	if err == nil {
		md.Register(minio)
	} else {
		zap.S().Errorw("failed to create minio downloader", "error", err)
	}

	// register the default downloader of the official RHCOS image.
	md.Register(iso.NewRHCOSHttpDownloader())

	if err := md.Download(ctx, out); err != nil {
		return err
	}

	return nil
}
