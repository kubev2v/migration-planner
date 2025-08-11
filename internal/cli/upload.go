package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gorm.io/gorm/utils"
)

type UploadOptions struct {
	GlobalOptions
	filePath string
	sourceId string
}

func DefaultUploadOptions() *UploadOptions {
	return &UploadOptions{
		GlobalOptions: DefaultGlobalOptions(),
	}
}

func NewCmdUpload() *cobra.Command {
	o := DefaultUploadOptions()
	cmd := &cobra.Command{
		Use:          "upload [rvtools|inventory]",
		Short:        "upload RVTools or inventory file",
		Example:      "upload rvtools --file-path /path/to/rvtools.xlsx --source-id <uuid>",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := o.Complete(cmd, args); err != nil {
				return err
			}
			if err := o.Validate(args); err != nil {
				return err
			}
			return o.Run(cmd.Context(), args)
		},
	}
	o.Bind(cmd.Flags())

	if err := validateFlags(cmd); err != nil {
		panic(err)
	}

	return cmd
}

func validateFlags(cmd *cobra.Command) error {
	requiredFlags := []string{"file-path", "source-id"}

	for _, flag := range requiredFlags {
		if err := cmd.MarkFlagRequired(flag); err != nil {
			return err
		}
	}

	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if utils.Contains(requiredFlags, f.Name) {
			f.Usage = fmt.Sprintf("%s (required)", f.Usage)
		}
	})

	return nil
}

func (o *UploadOptions) Bind(fs *pflag.FlagSet) {
	o.GlobalOptions.Bind(fs)

	fs.StringVar(&o.filePath, "file-path", o.filePath, "Path to the RVTools (.xlsx) or inventory (.json) file to upload")
	fs.StringVar(&o.sourceId, "source-id", o.sourceId, "UUID of the target source")
}

func (o *UploadOptions) Run(ctx context.Context, args []string) error {
	uploadType := args[0]

	switch uploadType {
	case "rvtools":
		return o.uploadRVTools(ctx)
	case "inventory":
		return o.uploadInventory(ctx)
	default:
		return fmt.Errorf("invalid upload type '%s'. Supported types: rvtools, inventory", uploadType)
	}
}

func (o *UploadOptions) uploadRVTools(ctx context.Context) error {
	c, err := o.Client()
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	f, err := os.Open(o.filePath)
	if err != nil {
		return fmt.Errorf("opening rvtools file: %w", err)
	}
	defer f.Close()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	part, err := mw.CreateFormFile("file", filepath.Base(o.filePath))
	if err != nil {
		return fmt.Errorf("creating form file: %w", err)
	}
	if _, err := io.Copy(part, f); err != nil {
		return fmt.Errorf("copying file into multipart: %w", err)
	}

	if err := mw.Close(); err != nil {
		return fmt.Errorf("closing multipart writer: %w", err)
	}

	sourceUUID, err := uuid.Parse(o.sourceId)
	if err != nil {
		return fmt.Errorf("parsing source UUID: %w", err)
	}

	resp, err := c.UploadRvtoolsFileWithBodyWithResponse(ctx, sourceUUID, mw.FormDataContentType(), &buf)
	if err != nil {
		return fmt.Errorf("error uploading multipart rvtools: %w", err)
	}
	if resp.JSON200 == nil {
		return fmt.Errorf("response body: %s", string(resp.Body))
	}

	fmt.Printf("\nRVTools file successfully uploaded\n")

	return nil
}

func (o *UploadOptions) uploadInventory(ctx context.Context) error {
	c, err := o.Client()
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	f, err := os.Open(o.filePath)
	if err != nil {
		return fmt.Errorf("opening rvtools file: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("reading file contents: %w", err)
	}

	var payload struct {
		Inventory v1alpha1.Inventory `json:"inventory"`
		Error     string             `json:"error"`
	}

	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("error unmarshalling inventory: %w", err)
	}

	sourceUUID, err := uuid.Parse(o.sourceId)
	if err != nil {
		return fmt.Errorf("parsing source UUID: %w", err)
	}

	body := v1alpha1.UpdateInventoryJSONRequestBody{
		Inventory: payload.Inventory,
	}

	resp, err := c.UpdateInventoryWithResponse(ctx, sourceUUID, body)
	if err != nil {
		return fmt.Errorf("error uploading inventory: %w", err)
	}

	if resp.JSON200 == nil {
		return fmt.Errorf("response body: %s", string(resp.Body))
	}

	fmt.Printf("\nInventory file successfully uploaded\n")

	return nil
}
