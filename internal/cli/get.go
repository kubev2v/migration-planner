package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"
	"text/tabwriter"

	"github.com/google/uuid"
	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	apiclient "github.com/kubev2v/migration-planner/internal/api/client"
	"github.com/kubev2v/migration-planner/internal/client"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/thoas/go-funk"
	"sigs.k8s.io/yaml"
)

const (
	jsonFormat = "json"
	yamlFormat = "yaml"
)

var (
	legalOutputTypes = []string{jsonFormat, yamlFormat}
)

type GetOptions struct {
	GlobalOptions

	Output string
}

func DefaultGetOptions() *GetOptions {
	return &GetOptions{
		GlobalOptions: DefaultGlobalOptions(),
	}
}

func NewCmdGet() *cobra.Command {
	o := DefaultGetOptions()
	cmd := &cobra.Command{
		Use:   "get (TYPE | TYPE/ID)",
		Short: "Display one or many resources.",
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

func (o *GetOptions) Bind(fs *pflag.FlagSet) {
	o.GlobalOptions.Bind(fs)

	fs.StringVarP(&o.Output, "output", "o", o.Output, fmt.Sprintf("Output format. One of: (%s).", strings.Join(legalOutputTypes, ", ")))
}

func (o *GetOptions) Complete(cmd *cobra.Command, args []string) error {
	if err := o.GlobalOptions.Complete(cmd, args); err != nil {
		return err
	}
	return nil
}

func (o *GetOptions) Validate(args []string) error {
	if err := o.GlobalOptions.Validate(args); err != nil {
		return err
	}

	_, _, err := parseAndValidateKindId(args[0])
	if err != nil {
		return err
	}

	if len(o.Output) > 0 && !funk.Contains(legalOutputTypes, o.Output) {
		return fmt.Errorf("output format must be one of %s", strings.Join(legalOutputTypes, ", "))
	}

	return nil
}

func (o *GetOptions) Run(ctx context.Context, args []string) error { // nolint: gocyclo
	c, err := client.NewFromConfigFile(o.ConfigFilePath)
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	var response interface{}

	kind, id, err := parseAndValidateKindId(args[0])
	if err != nil {
		return err
	}
	switch {
	case kind == SourceKind && id != nil:
		response, err = c.ReadSourceWithResponse(ctx, *id)
	case kind == SourceKind && id == nil:
		response, err = c.ListSourcesWithResponse(ctx)
	default:
		return fmt.Errorf("unsupported resource kind: %s", kind)
	}
	return processReponse(response, err, kind, id, o.Output)
}

func processReponse(response interface{}, err error, kind string, id *uuid.UUID, output string) error {
	errorPrefix := fmt.Sprintf("reading %s/%s", kind, id)
	if id == nil {
		errorPrefix = fmt.Sprintf("listing %s", plural(kind))
	}

	if err != nil {
		return fmt.Errorf(errorPrefix+": %w", err)
	}

	v := reflect.ValueOf(response).Elem()
	if v.FieldByName("HTTPResponse").Elem().FieldByName("StatusCode").Int() != http.StatusOK {
		return fmt.Errorf(errorPrefix+": %d", v.FieldByName("HTTPResponse").Elem().FieldByName("StatusCode").Int())
	}

	switch output {
	case jsonFormat:
		marshalled, err := json.Marshal(v.FieldByName("JSON200").Interface())
		if err != nil {
			return fmt.Errorf("marshalling resource: %w", err)
		}
		fmt.Printf("%s\n", string(marshalled))
		return nil
	case yamlFormat:
		marshalled, err := yaml.Marshal(v.FieldByName("JSON200").Interface())
		if err != nil {
			return fmt.Errorf("marshalling resource: %w", err)
		}
		fmt.Printf("%s\n", string(marshalled))
		return nil
	default:
		return printTable(response, kind, id)
	}
}

func printTable(response interface{}, kind string, id *uuid.UUID) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 8, 1, '\t', 0)
	switch {
	case kind == SourceKind && id == nil:
		printSourcesTable(w, *(response.(*apiclient.ListSourcesResponse).JSON200)...)
	case kind == SourceKind && id != nil:
		printSourcesTable(w, *(response.(*apiclient.ReadSourceResponse).JSON200))
	default:
		return fmt.Errorf("unknown resource type %s", kind)
	}
	w.Flush()
	return nil
}

func printSourcesTable(w *tabwriter.Writer, sources ...api.Source) {
	fmt.Fprintln(w, "ID")
	for _, s := range sources {
		fmt.Fprintf(w, "%s\n", s.Id)
	}
}
