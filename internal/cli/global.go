package cli

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/kubev2v/migration-planner/internal/api/client"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type GlobalOptions struct {
	ServerUrl string
	Token     string
	ProxyUrl  string
}

func DefaultGlobalOptions() GlobalOptions {
	return GlobalOptions{
		ServerUrl: "http://localhost:3443",
	}
}

func (o *GlobalOptions) Bind(fs *pflag.FlagSet) {
	fs.StringVarP(&o.ServerUrl, "server-url", "u", o.ServerUrl, "Address of the server")
	fs.StringVarP(&o.Token, "token", "", o.Token, "Token used to authenticate the user")
	fs.StringVar(&o.ProxyUrl, "proxy", "", "Address of the proxy")
}

func (o *GlobalOptions) Complete(cmd *cobra.Command, args []string) error {
	return nil
}

func (o *GlobalOptions) Validate(args []string) error {
	return nil
}

func (o *GlobalOptions) Client() (*client.ClientWithResponses, error) {
	httpClient := &http.Client{}

	if o.ProxyUrl != "" {
		if err := o.WithProxy(httpClient); err != nil {
			return nil, err
		}
	}

	return client.NewClientWithResponses(
		o.ServerUrl,
		client.WithHTTPClient(httpClient),
		client.WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
			if o.Token == "" {
				return nil
			}
			req.Header.Set("X-Authorization", fmt.Sprintf("Bearer %s", o.Token))
			return nil
		}),
	)
}

func (o *GlobalOptions) WithProxy(httpClient *http.Client) error {
	if !strings.HasPrefix(o.ProxyUrl, "http://") && !strings.HasPrefix(o.ProxyUrl, "https://") {
		o.ProxyUrl = "http://" + o.ProxyUrl
	}

	proxyUrl, err := url.Parse(o.ProxyUrl)
	if err != nil {
		return err
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyUrl),
	}

	httpClient.Transport = transport

	return nil
}
