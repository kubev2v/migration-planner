package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

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

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	if err := writer.WriteField("name", args[0]); err != nil {
		return fmt.Errorf("writing name field: %w", err)
	}

	part, err := writer.CreateFormFile("file", filepath.Base(o.excelFile))
	if err != nil {
		return fmt.Errorf("creating form file: %w", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("copying file content: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("closing multipart writer: %w", err)
	}

	response, err := c.CreateRVToolsAssessmentWithBodyWithResponse(ctx, writer.FormDataContentType(), body)
	if err != nil {
		return fmt.Errorf("failed to create assessment: %w", err)
	}

	if response.StatusCode() != http.StatusAccepted {
		if response.JSON400 != nil {
			return fmt.Errorf("failed to create assessment: %s", response.JSON400.Message)
		}
		if response.JSON500 != nil {
			return fmt.Errorf("failed to create assessment: %s", response.JSON500.Message)
		}
		return fmt.Errorf("failed to create assessment: %s", response.Status())
	}

	if response.JSON202 == nil {
		return fmt.Errorf("failed to create assessment: received 202 response but body is empty or malformed")
	}

	fmt.Printf("RVTools processing job started (ID: %d). The assessment will be created upon completion.\n", response.JSON202.Id)

	return nil
}
