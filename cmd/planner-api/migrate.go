package main

import (
	"context"

	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/pkg/log"
	"github.com/kubev2v/migration-planner/pkg/migrations"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate the db",
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := log.InitLog(zap.NewAtomicLevelAt(zap.InfoLevel))
		defer func() { _ = logger.Sync() }()

		undo := zap.ReplaceGlobals(logger)
		defer undo()

		cfg, err := config.New()
		if err != nil {
			zap.S().Fatalw("reading configuration", "error", err)
		}
		zap.S().Infow("Using config", "configuration", cfg)

		zap.S().Info("Initializing data store")
		db, err := store.InitDB(cfg)
		if err != nil {
			zap.S().Fatalw("initializing data store", "error", err)
		}

		store := store.NewStore(db)
		defer store.Close()

		zap.S().Info("Running database migrations")
		if err := migrations.MigrateStore(db, cfg.Service.MigrationFolder); err != nil {
			zap.S().Fatalw("running database migration", "error", err)
		}

		zap.S().Info("Running River migrations")
		if err := migrations.MigrateRiver(context.Background(), cfg); err != nil {
			zap.S().Fatalw("running River migration", "error", err)
		}

		return nil
	},
}
