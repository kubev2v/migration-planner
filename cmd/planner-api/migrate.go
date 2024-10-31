package main

import (
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/pkg/log"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate the db",
	RunE: func(cmd *cobra.Command, args []string) error {
		log := log.InitLogs()
		log.Println("Starting API service")
		defer log.Println("Db migrated")

		if configFile == "" {
			configFile = config.ConfigFile()
		}

		cfg, err := config.Load(configFile)
		if err != nil {
			log.Fatalf("reading configuration: %v", err)
		}
		log.Printf("Using config: %s", cfg)

		logLvl, err := logrus.ParseLevel(cfg.Service.LogLevel)
		if err != nil {
			logLvl = logrus.InfoLevel
		}
		log.SetLevel(logLvl)

		log.Println("Initializing data store")
		db, err := store.InitDB(cfg, log)
		if err != nil {
			log.Fatalf("initializing data store: %v", err)
		}

		store := store.NewStore(db, log.WithField("pkg", "store"))
		defer store.Close()

		if err := store.InitialMigration(); err != nil {
			log.Fatalf("running initial migration: %v", err)
		}

		return nil
	},
}
