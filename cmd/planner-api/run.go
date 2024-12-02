package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"

	apiserver "github.com/kubev2v/migration-planner/internal/api_server"
	"github.com/kubev2v/migration-planner/internal/api_server/agentserver"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/pkg/log"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the planner api",
	RunE: func(cmd *cobra.Command, args []string) error {
		zap.S().Info("Starting API service")
		defer zap.S().Info("API service stopped")

		if configFile == "" {
			configFile = config.ConfigFile()
		}
		cfg, err := config.LoadOrGenerate(configFile)
		if err != nil {
			zap.S().Fatalf("reading configuration: %v", err)
		}

		logLvl, err := zap.ParseAtomicLevel(cfg.Service.LogLevel)
		if err != nil {
			logLvl = zap.NewAtomicLevelAt(zapcore.InfoLevel)
		}

		logger := log.InitLog(logLvl)
		defer func() { _ = logger.Sync() }()

		undo := zap.ReplaceGlobals(logger)
		defer undo()

		zap.S().Info("Initializing data store")
		db, err := store.InitDB(cfg)
		if err != nil {
			zap.S().Fatalf("initializing data store: %v", err)
		}

		store := store.NewStore(db)
		defer store.Close()

		if err := store.InitialMigration(); err != nil {
			zap.S().Fatalf("running initial migration: %v", err)
		}

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGQUIT)
		go func() {
			defer cancel()
			listener, err := newListener(cfg.Service.Address)
			if err != nil {
				zap.S().Fatalf("creating listener: %s", err)
			}

			server := apiserver.New(cfg, store, listener)
			if err := server.Run(ctx); err != nil {
				zap.S().Fatalf("Error running server: %s", err)
			}
		}()

		go func() {
			defer cancel()
			listener, err := newListener(cfg.Service.AgentEndpointAddress)
			if err != nil {
				zap.S().Fatalf("creating listener: %s", err)
			}

			agentserver := agentserver.New(cfg, store, listener)
			if err := agentserver.Run(ctx); err != nil {
				zap.S().Fatalf("Error running server: %s", err)
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
