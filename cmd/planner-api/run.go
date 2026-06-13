package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kubev2v/migration-planner/pkg/opa"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/scram"

	"github.com/kubev2v/migration-planner/internal/api_server/agentserver"
	"github.com/kubev2v/migration-planner/internal/api_server/imageserver"
	"github.com/kubev2v/migration-planner/internal/rvtools/jobs"
	"github.com/kubev2v/migration-planner/pkg/metrics"

	apiserver "github.com/kubev2v/migration-planner/internal/api_server"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/service/eventwrap"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/pkg/events"
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
		defer func() { _ = store.Close() }()

		if err := migrations.MigrateStore(db, cfg.Service.MigrationFolder); err != nil {
			zap.S().Fatalw("running initial migration", "error", err)
		}

		zap.S().Info("Running River migrations")
		if err := migrations.MigrateRiver(context.Background(), cfg); err != nil {
			zap.S().Fatalw("running River migration", "error", err)
		}

		// The OpenShift Migration Advisor API expects the RHCOS ISO to be on disk
		if err := ensureIsoExist(cfg.Service.IsoPath); err != nil {
			zap.S().Fatalw("validate iso", "error", err)
			return err
		}

		// Initialize OPA validator for policy validation
		zap.S().Info("initializing OPA validator...")
		opaValidator, err := opa.NewValidatorFromDir(cfg.Service.OpaPoliciesFolder)
		if err != nil {
			zap.S().Fatalw("initialize OPA validator", "error", err)
		}

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGQUIT)
		var wg sync.WaitGroup // Responsible for keeping the main thread waiting for all goroutines to shut down gracefully

		// Create Kafka producer and event writer
		var writer events.Writer = events.NewNoOpWriter()

		if cfg.Kafka.Enabled {
			var cleanup func()
			writer, cleanup, err = createEventWriter(ctx, cfg)
			if err != nil {
				zap.S().Warnw("failed to create kafka producer", "error", err)
			}
			defer cleanup()
			zap.S().Info("Kafka writer initialized")
		}

		// Start outbox dispatcher
		dispatcher := eventwrap.NewOutboxDispatcher(store, writer, 5*time.Second)
		wg.Add(1)
		go func() {
			defer wg.Done()
			dispatcher.Run(ctx)
		}()
		zap.S().Info("Outbox dispatcher started")

		// Create pgx pool for River and RVTools file storage
		zap.S().Info("Initializing River jobs client...")
		pool, err := jobs.CreatePgxPool(ctx, cfg)
		if err != nil {
			zap.S().Fatalw("creating pgx pool", "error", err)
		}

		jobsClient, err := jobs.NewClient(pool, store, opaValidator)
		if err != nil {
			zap.S().Fatalw("initializing River jobs client", "error", err)
		}
		if err := jobsClient.RiverClient.Start(context.Background()); err != nil {
			zap.S().Fatalw("starting River jobs client", "error", err)
		}
		zap.S().Info("River jobs client started")

		// Ensure cleanup on function exit
		defer func() {
			zap.S().Info("Stopping River jobs client...")
			if err := jobsClient.Stop(context.Background()); err != nil {
				zap.S().Warnf("Error stopping River jobs client: %v", err)
			}
		}()

		// register metrics
		metrics.RegisterMetrics(store)

		runServer(ctx, &wg, cancel, cfg.Service.Address, "api_server", func(l net.Listener) Server {
			return apiserver.New(cfg, store, l, opaValidator, jobsClient)
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

func createEventWriter(ctx context.Context, cfg *config.Config) (events.Writer, func(), error) {
	brokers := strings.Split(cfg.Kafka.Brokers, ",")
	var kafkaOpts []kgo.Opt

	if cfg.Kafka.UseTLS {
		kafkaOpts = append(kafkaOpts, kgo.DialTLSConfig(new(tls.Config)))
	}

	if cfg.Kafka.SASLUsername != "" {
		mechanism := scram.Auth{
			User: cfg.Kafka.SASLUsername,
			Pass: cfg.Kafka.SASLPassword,
		}
		kafkaOpts = append(kafkaOpts, kgo.SASL(mechanism.AsSha512Mechanism()))
	}

	noop := func() {}

	producer, err := events.NewKafkaProducer(brokers, kafkaOpts...)
	if err != nil {
		return events.NewNoOpWriter(), noop, err
	}

	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pingCancel()

	if err := producer.Ping(pingCtx); err != nil {
		producer.Close()
		return events.NewNoOpWriter(), noop, fmt.Errorf("kafka broker unreachable: %w", err)
	}

	zap.S().Infow("kafka producer connected", "brokers", cfg.Kafka.Brokers)

	return producer, producer.Close, nil
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
