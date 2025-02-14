package cli

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/client"
	"github.com/kubev2v/migration-planner/internal/image"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type GenerateOptions struct {
	GlobalOptions
	ImageType           string
	AgentImageURL       string
	ServiceIP           string
	OutputImageFilePath string
}

func DefaultGenerateOptions() *GenerateOptions {
	return &GenerateOptions{
		GlobalOptions: DefaultGlobalOptions(),
		ImageType:     "ova",
		AgentImageURL: "quay.io/kubev2v/migration-planner-agent:latest",
	}
}

func (o *GenerateOptions) Complete(cmd *cobra.Command, args []string) error {
	return nil
}

func (o *GenerateOptions) Validate(args []string) error {
	if _, err := uuid.Parse(args[0]); err != nil {
		return fmt.Errorf("invalid source id: %s", err)
	}

	if o.AgentImageURL == "" {
		return fmt.Errorf("agent image url is invalid")
	}

	if o.OutputImageFilePath == "" {
		return fmt.Errorf("output image is empty")
	}

	if o.ServiceIP == "" {
		localIP, err := getLocalIP()
		if err != nil {
			return fmt.Errorf("failed to get local ip: %s. Please provide planner api ip", err)
		}
		o.ServiceIP = fmt.Sprintf("http://%s:7443", localIP.String())
	}

	switch o.ImageType {
	case "ova":
		fallthrough
	case "iso":
		return nil
	default:
		return fmt.Errorf("image type must be either ova or iso")
	}
}

func NewCmdGenerate() *cobra.Command {
	o := DefaultGenerateOptions()
	cmd := &cobra.Command{
		Use:     "generate SOURCE_ID [FLAGS]",
		Short:   "Generate an image",
		Example: "generate some-source-id -t iso -o path_to_image_file",
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

func (o *GenerateOptions) Bind(fs *pflag.FlagSet) {
	fs.StringVarP(&o.ImageType, "image-type", "t", "ova", "Type of the image. Only accepts ova and iso")
	fs.StringVarP(&o.AgentImageURL, "agent-image-url", "u", "quay.io/kubev2v/migration-planner-agent:latest", "Quay url of the agent's image. Defaults to quay.io/kubev2v/migration-planner-agent:latest")
	fs.StringVarP(&o.OutputImageFilePath, "output-file", "o", "", "Output image file path")
}

func (o *GenerateOptions) Run(ctx context.Context, args []string) error {
	c, err := client.NewFromConfigFile(o.ConfigFilePath)
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	resp, err := c.GetSourceWithResponse(ctx, uuid.MustParse(args[0]))
	if err != nil {
		return fmt.Errorf("failed to get source %q: %s", args[0], err)
	}

	if resp.JSON200 == nil {
		return fmt.Errorf("failed to get source %q: %s", args[0], err)
	}

	source := *resp.JSON200

	imageBuilder := image.NewImageBuilder(source.Id).WithPlannerAgentImage(o.AgentImageURL).WithPlannerService(o.ServiceIP)

	switch o.ImageType {
	case "iso":
		imageBuilder = imageBuilder.WithImageType(image.QemuImageType).WithPersistenceDiskDevice("/dev/vda")
	default:
	}

	output, err := os.Create(o.OutputImageFilePath)
	if err != nil {
		return err
	}

	if _, err := imageBuilder.Generate(ctx, output); err != nil {
		return fmt.Errorf("failed to write image: %s", err)
	}

	fmt.Printf("Image wrote to %s\n", o.OutputImageFilePath)

	return nil
}

func getLocalIP() (net.IP, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP, nil
}
