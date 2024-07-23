package cli

import (
	"context"
	"fmt"
	"net/http"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/client"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type CreateOptions struct {
	GlobalOptions
}

func DefaultCreateOptions() *CreateOptions {
	return &CreateOptions{
		GlobalOptions: DefaultGlobalOptions(),
	}
}

func NewCmdCreate() *cobra.Command {
	o := DefaultCreateOptions()
	cmd := &cobra.Command{
		Use:   "create TYPE NAME",
		Short: "Create a resource.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := o.Complete(cmd, args); err != nil {
				return err
			}
			if err := o.Validate(args); err != nil {
				return err
			}
			return o.Run(cmd.Context(), args)
		},
		SilenceUsage: true,
	}
	o.Bind(cmd.Flags())
	return cmd
}

func (o *CreateOptions) Bind(fs *pflag.FlagSet) {
	o.GlobalOptions.Bind(fs)
}

func (o *CreateOptions) Complete(cmd *cobra.Command, args []string) error {
	if err := o.GlobalOptions.Complete(cmd, args); err != nil {
		return err
	}

	return nil
}

func (o *CreateOptions) Validate(args []string) error {
	if err := o.GlobalOptions.Validate(args); err != nil {
		return err
	}

	_, _, err := parseAndValidateKindId(args[0])
	if err != nil {
		return err
	}

	return nil
}

func (o *CreateOptions) Run(ctx context.Context, args []string) error {
	c, err := client.NewFromConfigFile(o.ConfigFilePath)
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	body := api.SourceCreate{Name: args[1]}
	response, err := c.CreateSource(ctx, body)
	if err != nil {
		return fmt.Errorf("creating source: %w, http response: %+v", err, response)
	}
	if response.StatusCode != http.StatusCreated {
		return fmt.Errorf("creating source: %+v", response)
	}
	return nil
}
