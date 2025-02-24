package main

import (
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/pkg/log"
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

		envConfig, err := config.New()
		if err != nil {
			zap.S().Fatalf("reading configuration: %v", err)
		}
		zap.S().Infof("Using config: %s", envConfig)

		zap.S().Info("Initializing data store")
		db, err := store.InitDB(&envConfig.DB)
		if err != nil {
			zap.S().Fatalf("initializing data store: %v", err)
		}

		store := store.NewStore(db)
		defer store.Close()

		if err := store.InitialMigration(); err != nil {
			zap.S().Fatalf("running initial migration: %v", err)
		}

		return nil
	},
}
