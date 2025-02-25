package cli

import (
	"context"
	"net/http"

	"github.com/kubev2v/migration-planner/internal/api/client"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type GlobalOptions struct {
	ServerUrl string
}

func DefaultGlobalOptions() GlobalOptions {
	return GlobalOptions{
		ServerUrl: "http://localhost:3443",
	}
}

func (o *GlobalOptions) Bind(fs *pflag.FlagSet) {
	fs.StringVarP(&o.ServerUrl, "server-url", "u", o.ServerUrl, "Address of the server")
}

func (o *GlobalOptions) Complete(cmd *cobra.Command, args []string) error {
	return nil
}

func (o *GlobalOptions) Validate(args []string) error {
	return nil
}

func (o *GlobalOptions) Client() (*client.ClientWithResponses, error) {
	return client.NewClientWithResponses(
		o.ServerUrl,
		client.WithHTTPClient(&http.Client{}),
		client.WithRequestEditorFn(func(ctx context.Context, req *http.Request) error { return nil }),
	)
}
