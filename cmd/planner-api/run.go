package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/IBM/sarama"
	pkgKafka "github.com/kubev2v/migration-event-streamer/pkg/kafka"
	apiserver "github.com/kubev2v/migration-planner/internal/api_server"
	"github.com/kubev2v/migration-planner/internal/api_server/agentserver"
	"github.com/kubev2v/migration-planner/internal/api_server/imageserver"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/events"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/pkg/log"
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

		envConfig, err := config.New()
		if err != nil {
			zap.S().Fatalf("reading environment options: %v", err)
		}

		logLvl, err := zap.ParseAtomicLevel(envConfig.Svc.LogLevel)
		if err != nil {
			logLvl = zap.NewAtomicLevelAt(zapcore.InfoLevel)
		}

		logger := log.InitLog(logLvl)
		defer func() { _ = logger.Sync() }()

		undo := zap.ReplaceGlobals(logger)
		defer undo()

		zap.S().Info("Starting API service...")
		zap.S().Infof("Build from git commit: %s", version.Get().GitCommit)
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

		// Initialize database with basic example report
		if v, b := os.LookupEnv("NO_SEED"); !b || v == "false" {
			if err := store.Seed(); err != nil {
				zap.S().Fatalf("seeding database with default report: %v", err)
			}
		}

		// initilize event writer
		ep, _ := getEventProducer(envConfig.Kafka)

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGQUIT)
		go func() {
			defer cancel()
			listener, err := newListener(envConfig.Svc.Address)
			if err != nil {
				zap.S().Fatalf("creating listener: %s", err)
			}

			server := apiserver.New(&envConfig.Svc, envConfig.Auth, store, ep, listener)
			if err := server.Run(ctx); err != nil {
				zap.S().Fatalf("Error running server: %s", err)
			}
		}()

		go func() {
			defer cancel()
			listener, err := newListener(envConfig.Svc.AgentEndpointAddress)
			if err != nil {
				zap.S().Fatalf("creating listener: %s", err)
			}

			agentserver := agentserver.New(&envConfig.Svc, envConfig.Auth, store, ep, listener)
			if err := agentserver.Run(ctx); err != nil {
				zap.S().Fatalf("Error running server: %s", err)
			}
		}()

		go func() {
			defer cancel()
			listener, err := newListener(envConfig.Svc.ImageEndpointAddress)
			if err != nil {
				zap.S().Fatalf("creating listener: %s", err)
			}

			imageserver := imageserver.New(&envConfig.Svc, store, ep, listener)
			if err := imageserver.Run(ctx); err != nil {
				zap.S().Fatalf("Error running server: %s", err)
			}
		}()

		go func() {
			defer cancel()
			listener, err := newListener("0.0.0.0:8080")
			if err != nil {
				zap.S().Named("metrics_server").Fatalf("creating listener: %s", err)
			}
			metricsServer := apiserver.NewMetricServer("0.0.0.0:8080", listener)
			if err := metricsServer.Run(ctx); err != nil {
				zap.S().Named("metrics_server").Fatalf("failed to run metrics server: %s", err)
			}
		}()

		<-ctx.Done()
		_ = ep.Close()

		return nil
	},
}

func newListener(address string) (net.Listener, error) {
	if address == "" {
		address = "localhost:0"
	}
	return net.Listen("tcp", address)
}

func getEventProducer(kafkaCfg config.KafkaConfig) (*events.EventProducer, error) {
	if len(kafkaCfg.Brokers) == 0 {
		stdWriter := &events.StdoutWriter{}
		ew := events.NewEventProducer(stdWriter)
		return ew, nil
	}

	saramaConfig := sarama.NewConfig()
	if kafkaCfg.SaramaConfig != nil {
		saramaConfig = kafkaCfg.SaramaConfig
	}
	saramaConfig.Version = sarama.V3_6_0_0

	kp, err := pkgKafka.NewKafkaProducer(kafkaCfg.Brokers, saramaConfig)
	if err != nil {
		return nil, err
	}

	zap.S().Named("planner-api").Infof("connected to kafka: %v", kafkaCfg.Brokers)

	if kafkaCfg.Topic != "" {
		return events.NewEventProducer(kp, events.WithOutputTopic(kafkaCfg.Topic)), nil
	}

	return events.NewEventProducer(kp), nil
}
