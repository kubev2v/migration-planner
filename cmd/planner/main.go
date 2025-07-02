package main

import (
	"github.com/kubev2v/migration-planner/internal/cli/standalone"
	"go.uber.org/zap"
	"os"

	"github.com/kubev2v/migration-planner/internal/cli"
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

	command := NewPlannerCtlCommand()
	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}

func NewPlannerCtlCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "planner [flags] [options]",
		Short: "planner controls the Migration Planner service.",
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
			os.Exit(1)
		},
	}
	cmd.AddCommand(cli.NewCmdGet())
	cmd.AddCommand(cli.NewCmdDelete())
	cmd.AddCommand(cli.NewCmdVersion())
	cmd.AddCommand(cli.NewCmdCreate())
	cmd.AddCommand(cli.NewCmdGenerate())
	cmd.AddCommand(cli.NewCmdDeploy())
	cmd.AddCommand(cli.NewCmdSSO())
	cmd.AddCommand(cli.NewCmdE2E())
	cmd.AddCommand(standalone.NewCmdCollect())

	return cmd
}
