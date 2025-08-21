package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/kubev2v/migration-planner/internal/api_server/isoserver"
	"github.com/kubev2v/migration-planner/pkg/log"
	"github.com/spf13/pflag"
	"go.uber.org/zap/zapcore"

	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/pkg/iso"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

type InitOptions struct {
	Serve bool
	Port  string
}

func DefaultInitOptions() *InitOptions {
	return &InitOptions{
		Serve: false,
	}
}

func NewCmdInit() *cobra.Command {
	o := DefaultInitOptions()
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize RHCOS ISO for the migration planner",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.New()
			if err != nil {
				return fmt.Errorf("reading config: %w", err)
			}
			_, undo := setupLogger(cfg.Service.LogLevel)
			defer undo()
			zap.S().Info("Starting ISO initialization...")

			ctx, cancel := setupSignalContext()
			defer cancel()

			// Initialize ISOs
			zap.S().Info("Initializing RHCOS ISO")
			isoInitializer := newIsoInitializer(cfg)
			if err := isoInitializer.Initialize(ctx, cfg.Service.IsoPath, cfg.Service.RhcosImageSha256); err != nil {
				zap.S().Fatalw("failed to initialize iso", "error", err)
			}

			if o.Serve {
				return serveISO(ctx, o.Port, cfg.Service.IsoPath)
			}
			return nil
		},
	}

	cmd.SilenceUsage = true
	o.Bind(cmd.Flags())
	return cmd
}

func setupLogger(levelStr string) (*zap.Logger, func()) {
	level, err := zap.ParseAtomicLevel(levelStr)
	if err != nil {
		level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	}
	logger := log.InitLog(level)
	undo := zap.ReplaceGlobals(logger)
	return logger, func() { _ = logger.Sync(); undo() }
}

func setupSignalContext() (context.Context, func()) {
	ctx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt, syscall.SIGHUP,
		syscall.SIGTERM, syscall.SIGQUIT,
	)
	return ctx, cancel
}

func serveISO(ctx context.Context, port, isoPath string) error {
	addr := fmt.Sprintf("0.0.0.0:%s", port)
	ln, err := newListener(addr)
	if err != nil {
		return fmt.Errorf("creating listener: %w", err)
	}
	server := isoserver.New(ln, addr, isoPath)
	return server.Run(ctx)
}

func (o *InitOptions) Bind(fs *pflag.FlagSet) {
	fs.BoolVar(&o.Serve, "listen", false, "listen to serve the iso file")
	fs.StringVarP(&o.Port, "port", "p", "8080", "Port to listen on")
}

func newIsoInitializer(cfg *config.Config) *iso.IsoInitializer {
	md := iso.NewDownloaderManager()

	minio, err := iso.NewMinioDownloader(
		iso.WithEndpoint(cfg.Service.S3.Endpoint),
		iso.WithBucket(cfg.Service.S3.Bucket),
		iso.WithAccessKey(cfg.Service.S3.AccessKey),
		iso.WithSecretKey(cfg.Service.S3.SecretKey),
		iso.WithImage(cfg.Service.S3.IsoFileName, cfg.Service.RhcosImageSha256),
	)
	if err == nil {
		md.Register(minio)
	} else {
		zap.S().Errorw("failed to create minio downloader", "error", err)
	}

	// register the default downloader of the official RHCOS image.
	md.Register(iso.NewHttpDownloader(cfg.Service.RhcosImageName, cfg.Service.RhcosImageSha256))

	return iso.NewIsoInitializer(md)
}
