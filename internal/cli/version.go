package cli

import (
	"context"
	"fmt"

	"github.com/kubev2v/migration-planner/pkg/version"
	"github.com/spf13/cobra"
)

type VersionOptions struct {
	Output string
}

func DefaultVersionOptions() *VersionOptions {
	return &VersionOptions{
		Output: "",
	}
}

func NewCmdVersion() *cobra.Command {
	o := DefaultVersionOptions()
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print Planner version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			return o.Run(cmd.Context(), args)
		},
	}
	return cmd
}

func (o *VersionOptions) Run(ctx context.Context, args []string) error {
	versionInfo := version.Get()
	fmt.Printf("Planner Version: %s\n", versionInfo.String())
	return nil
}
