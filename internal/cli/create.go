package cli

import (
	"github.com/spf13/cobra"
)

func NewCmdCreate() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a resource",
	}
	cmd.AddCommand(NewCmdCreateSource())
	cmd.AddCommand(NewCmdCreateAssessment())
	return cmd
}
