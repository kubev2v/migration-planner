package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type DeleteOptions struct {
	GlobalOptions
}

func DefaultDeleteOptions() *DeleteOptions {
	return &DeleteOptions{
		GlobalOptions: DefaultGlobalOptions(),
	}
}

func NewCmdDelete() *cobra.Command {
	o := DefaultDeleteOptions()
	cmd := &cobra.Command{
		Use:   "delete (TYPE | TYPE/NAME)",
		Short: "Delete resources by resources or owner.",
		Args:  cobra.ExactArgs(1),
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

func (o *DeleteOptions) Bind(fs *pflag.FlagSet) {
	o.GlobalOptions.Bind(fs)
}

func (o *DeleteOptions) Complete(cmd *cobra.Command, args []string) error {
	if err := o.GlobalOptions.Complete(cmd, args); err != nil {
		return err
	}

	return nil
}

func (o *DeleteOptions) Validate(args []string) error {
	if err := o.GlobalOptions.Validate(args); err != nil {
		return err
	}

	_, _, err := parseAndValidateKindId(args[0])
	if err != nil {
		return err
	}
	return nil
}

func (o *DeleteOptions) Run(ctx context.Context, args []string) error {
	c, err := o.Client()
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	kind, id, err := parseAndValidateKindId(args[0])
	if err != nil {
		return err
	}
	switch kind {
	case SourceKind:
		if id != nil {
			response, err := c.DeleteSourceWithResponse(ctx, *id)
			if err != nil {
				return fmt.Errorf("deleting %s/%s: %w", kind, id, err)
			}
			fmt.Printf("%s\n", response.Status())
		} else {
			response, err := c.DeleteSourcesWithResponse(ctx)
			if err != nil {
				return fmt.Errorf("deleting %s: %w", plural(kind), err)
			}
			fmt.Printf("%s\n", response.Status())
		}
	default:
		return fmt.Errorf("unsupported resource kind: %s", kind)
	}

	return nil
}
