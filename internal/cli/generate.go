package cli

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/image"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type GenerateOptions struct {
	GlobalOptions
	RHCOSImage          string
	ImageType           string
	AgentImageURL       string
	ServiceIP           string
	OutputImageFilePath string
	HttpProxyUrl        string
	HttpsProxyUrl       string
	NoProxyDomain       string
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
			return fmt.Errorf("failed to get local ip: %s, Please provide planner api ip", err)
		}
		o.ServiceIP = fmt.Sprintf("http://%s:7443", localIP.String())
	}

	if o.RHCOSImage != "" {
		rhcosFile, err := os.Open(o.RHCOSImage)
		if err != nil {
			return fmt.Errorf("failed to open rhcos base image file: %s", err)
		}
		rhcosFile.Close()
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
	o.GlobalOptions.Bind(fs)

	fs.StringVarP(&o.ImageType, "image-type", "", "ova", "Type of the image. Only accepts ova and iso")
	fs.StringVarP(&o.AgentImageURL, "agent-image-url", "", "quay.io/kubev2v/migration-planner-agent:latest", "Quay url of the agent's image. Defaults to quay.io/kubev2v/migration-planner-agent:latest")
	fs.StringVarP(&o.OutputImageFilePath, "output-file", "", "", "Output image file path")
	fs.StringVarP(&o.HttpProxyUrl, "http-proxy", "", "", "Url of HTTP_PROXY")
	fs.StringVarP(&o.HttpsProxyUrl, "https-proxy", "", "", "Url of HTTPS_PROXY")
	fs.StringVarP(&o.NoProxyDomain, "no-proxy", "", "", "list of domains without proxy")
	fs.StringVarP(&o.RHCOSImage, "rhcos-base-image", "", "", "path to the rhcos base image")
}

func (o *GenerateOptions) Run(ctx context.Context, args []string) error {
	c, err := o.Client()
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

	imageBuilder := image.NewImageBuilder(source.Id).
		WithPlannerService(o.ServiceIP).
		WithProxy(image.Proxy{
			HttpUrl:       o.HttpProxyUrl,
			HttpsUrl:      o.HttpsProxyUrl,
			NoProxyDomain: o.NoProxyDomain,
		})

	switch o.ImageType {
	case "iso":
		imageBuilder = imageBuilder.
			WithImageType(image.QemuImageType).
			WithPersistenceDiskDevice("/dev/vda")
		if o.RHCOSImage != "" {
			imageBuilder = imageBuilder.WithRHCOSImage(o.RHCOSImage)
		}
	default:
	}

	output, err := os.Create(o.OutputImageFilePath)
	if err != nil {
		return err
	}

	if err := imageBuilder.Generate(ctx, output); err != nil {
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
