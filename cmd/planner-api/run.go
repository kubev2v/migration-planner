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
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the planner api",
	RunE: func(cmd *cobra.Command, args []string) error {
		log := log.InitLogs()
		log.Println("Starting API service")
		defer log.Println("API service stopped")

		if configFile == "" {
			configFile = config.ConfigFile()
		}
		cfg, err := config.LoadOrGenerate(configFile)
		if err != nil {
			log.Fatalf("reading configuration: %v", err)
		}

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

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGQUIT)
		go func() {
			defer cancel()
			listener, err := newListener(cfg.Service.Address)
			if err != nil {
				log.Fatalf("creating listener: %s", err)
			}

			server := apiserver.New(log, cfg, store, listener)
			if err := server.Run(ctx); err != nil {
				log.Fatalf("Error running server: %s", err)
			}
		}()

		go func() {
			defer cancel()
			listener, err := newListener(cfg.Service.AgentEndpointAddress)
			if err != nil {
				log.Fatalf("creating listener: %s", err)
			}

			agentserver := agentserver.New(log, cfg, store, listener)
			if err := agentserver.Run(ctx); err != nil {
				log.Fatalf("Error running server: %s", err)
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
