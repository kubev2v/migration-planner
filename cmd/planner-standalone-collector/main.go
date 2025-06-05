package main

import (
	"github.com/kubev2v/migration-planner/internal/cli/standalone"
	"go.uber.org/zap"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = logger.Sync()
	}()
	zap.ReplaceGlobals(logger)

	command := newStandaloneCollector()
	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}

func newStandaloneCollector() *cobra.Command {
	return standalone.NewCmdCollect()
}
