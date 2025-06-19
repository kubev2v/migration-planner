package main

import (
	"github.com/kubev2v/migration-planner/internal/cli/standalone"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	command := newStandaloneCollector()
	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}

func newStandaloneCollector() *cobra.Command {
	return standalone.NewCmdCollect()
}
