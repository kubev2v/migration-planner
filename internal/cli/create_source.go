package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type CreateSourceOptions struct {
	GlobalOptions

	SshKeyFile string
}

func DefaultCreateSourceOptions() *CreateSourceOptions {
	return &CreateSourceOptions{
		GlobalOptions: DefaultGlobalOptions(),
	}
}

func NewCmdCreateSource() *cobra.Command {
	o := DefaultCreateSourceOptions()
	cmd := &cobra.Command{
		Use:     "source NAME",
		Short:   "Create a source",
		Example: "create source new-source -s ~/.ssh/some_key.pub",
		Args:    cobra.ExactArgs(1),
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

func (o *CreateSourceOptions) Bind(fs *pflag.FlagSet) {
	o.GlobalOptions.Bind(fs)

	fs.StringVarP(&o.SshKeyFile, "sshkey-file", "s", o.SshKeyFile, "Path to the ssh key")
}

func (o *CreateSourceOptions) Run(ctx context.Context, args []string) error {
	c, err := o.Client()
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	params := v1alpha1.SourceCreate{
		Name: args[0],
	}
	if o.SshKeyFile != "" {
		sshKey, err := os.ReadFile(o.SshKeyFile)
		if err != nil {
			return fmt.Errorf("failed to read ssh key %s: %s", o.SshKeyFile, err)
		}
		key := string(sshKey)
		params.SshPublicKey = &key
	}

	response, err := c.CreateSourceWithResponse(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to create source: %w", err)
	}

	if response.StatusCode() != http.StatusCreated {
		return fmt.Errorf("failed to create source: %s", response.Status())
	}

	fmt.Println(response.JSON201.Id)

	return nil
}
