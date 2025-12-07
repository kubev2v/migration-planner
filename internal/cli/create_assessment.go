package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/rvtools"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type CreateAssessmentOptions struct {
	GlobalOptions

	excelFile string
}

func DefaultCreateAssessmentOptions() *CreateAssessmentOptions {
	return &CreateAssessmentOptions{
		GlobalOptions: DefaultGlobalOptions(),
	}
}

func NewCmdCreateAssessment() *cobra.Command {
	o := DefaultCreateAssessmentOptions()
	cmd := &cobra.Command{
		Use:   "assessment NAME",
		Short: "Create an assessment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := o.Complete(cmd, args); err != nil {
				return err
			}
			if err := o.GlobalOptions.Validate(args); err != nil {
				return err
			}
			return o.Run(cmd.Context(), args)
		},
		SilenceUsage: true,
	}

	o.Bind(cmd.Flags())
	return cmd
}

func (o *CreateAssessmentOptions) Bind(fs *pflag.FlagSet) {
	o.GlobalOptions.Bind(fs)

	fs.StringVarP(&o.excelFile, "file", "f", o.excelFile, "Path to the Rvtools .xlsx file")
}

func (o *CreateAssessmentOptions) Run(ctx context.Context, args []string) error {
	if o.excelFile == "" {
		return fmt.Errorf("must specify an Excel file")
	}

	c, err := o.Client()
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	file, err := os.Open(o.excelFile)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	inventoryData, err := rvtools.ParseRVTools(ctx, data, nil)
	if err != nil {
		return fmt.Errorf("parsing inventory file: %w", err)
	}

	inv := &v1alpha1.Inventory{}
	if err := json.Unmarshal(inventoryData, inv); err != nil {
		return fmt.Errorf("unmarshal inventory: %w", err)
	}

	params := v1alpha1.AssessmentForm{
		Name:       args[0],
		Inventory:  inv,
		SourceType: "rvtools",
	}

	response, err := c.CreateAssessmentWithResponse(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to create assessment: %w", err)
	}

	if response.StatusCode() != http.StatusCreated {
		return fmt.Errorf("failed to create assessment: %s", response.Status())
	}

	fmt.Println(response.JSON201.Id)

	return nil
}
