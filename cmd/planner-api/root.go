package main

import "github.com/spf13/cobra"

var (
	configFile string
)

var rootCmd = &cobra.Command{
	Use: "planner-api",
}

func init() {
	rootCmd.AddCommand(migrateCmd)
	rootCmd.AddCommand(runCmd)

	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "Path to configuration file")
}
