package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/client"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type CreateOptions struct {
	GlobalOptions

	SshKeyFile string
}

func DefaultCreateOptions() *CreateOptions {
	return &CreateOptions{
		GlobalOptions: DefaultGlobalOptions(),
	}
}

func NewCmdCreate() *cobra.Command {
	o := DefaultCreateOptions()
	cmd := &cobra.Command{
		Use:     "create NAME",
		Short:   "Create a source",
		Example: "create new-source -s ~/.ssh/some_key.pub",
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

func (o *CreateOptions) Bind(fs *pflag.FlagSet) {
	o.GlobalOptions.Bind(fs)

	fs.StringVarP(&o.SshKeyFile, "sshkey-file", "s", o.SshKeyFile, "Path to the ssh key")
}

func (o *CreateOptions) Run(ctx context.Context, args []string) error {
	c, err := client.NewFromConfigFile(o.ConfigFilePath)
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
