package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/kubev2v/migration-planner/pkg/version"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/thoas/go-funk"
	"sigs.k8s.io/yaml"
)

type InfoOptions struct {
	GlobalOptions
	Output string
	Remote bool
}

func DefaultInfoOptions() *InfoOptions {
	return &InfoOptions{
		GlobalOptions: DefaultGlobalOptions(),
		Output:        "",
		Remote:        false,
	}
}

func NewCmdInfo() *cobra.Command {
	o := DefaultInfoOptions()
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Print Planner information",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := o.Complete(cmd, args); err != nil {
				return err
			}
			if err := o.Validate(); err != nil {
				return err
			}
			return o.Run(cmd.Context(), args)
		},
	}
	o.Bind(cmd.Flags())
	return cmd
}

func (o *InfoOptions) Bind(fs *pflag.FlagSet) {
	o.GlobalOptions.Bind(fs)
	fs.StringVarP(&o.Output, "output", "o", o.Output, fmt.Sprintf("Output format. One of: (%s).", strings.Join(legalOutputTypes, ", ")))
	fs.BoolVar(&o.Remote, "remote", o.Remote, "Get information from the remote service")
}

func (o *InfoOptions) Complete(cmd *cobra.Command, args []string) error {
	return o.GlobalOptions.Complete(cmd, args)
}

func (o *InfoOptions) Validate() error {
	if err := o.GlobalOptions.Validate([]string{}); err != nil {
		return err
	}
	if len(o.Output) > 0 && !funk.Contains(legalOutputTypes, o.Output) {
		return fmt.Errorf("output format must be one of %s", strings.Join(legalOutputTypes, ", "))
	}
	return nil
}

// InfoResponse represents the information we want to display
type InfoResponse struct {
	GitCommit   string `json:"gitCommit" yaml:"gitCommit"`
	VersionName string `json:"versionName" yaml:"versionName"`
}

func (o *InfoOptions) Run(ctx context.Context, args []string) error {
	var info InfoResponse
	var err error

	if o.Remote {
		// Get information from remote service
		info, err = o.getRemoteInfo(ctx)
		if err != nil {
			return fmt.Errorf("failed to get remote info: %w", err)
		}
	} else {
		// Get local information
		versionInfo := version.Get()
		info = InfoResponse{
			GitCommit:   versionInfo.GitCommit,
			VersionName: versionInfo.GitVersion,
		}
	}

	return o.printInfo(info)
}

func (o *InfoOptions) getRemoteInfo(ctx context.Context) (InfoResponse, error) {
	client, err := o.Client()
	if err != nil {
		return InfoResponse{}, fmt.Errorf("creating client: %w", err)
	}

	response, err := client.GetInfoWithResponse(ctx)
	if err != nil {
		return InfoResponse{}, fmt.Errorf("calling remote info endpoint: %w", err)
	}

	if response.HTTPResponse.StatusCode != http.StatusOK {
		return InfoResponse{}, fmt.Errorf("remote service returned status: %d", response.HTTPResponse.StatusCode)
	}

	if response.JSON200 == nil {
		return InfoResponse{}, fmt.Errorf("empty response from remote service")
	}

	return InfoResponse{
		GitCommit:   response.JSON200.GitCommit,
		VersionName: response.JSON200.VersionName,
	}, nil
}

func (o *InfoOptions) printInfo(info InfoResponse) error {
	switch o.Output {
	case jsonFormat:
		data, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal info to JSON: %w", err)
		}
		fmt.Println(string(data))
	case yamlFormat:
		data, err := yaml.Marshal(info)
		if err != nil {
			return fmt.Errorf("failed to marshal info to YAML: %w", err)
		}
		fmt.Print(string(data))
	default:
		// Default human-readable format
		source := "Local CLI"
		if o.Remote {
			source = "Remote Service"
		}
		fmt.Printf("Migration Planner %s Information:\n", source)
		fmt.Printf("  Version Name: %s\n", info.VersionName)
		fmt.Printf("  Git Commit:   %s\n", info.GitCommit)
	}

	return nil
}
