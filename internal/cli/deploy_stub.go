//go:build !libvirt

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewCmdDeploy() *cobra.Command {
	return &cobra.Command{
		Use:   "deploy",
		Short: "Deploy an agent (requires libvirt)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("deploy command requires libvirt support. Please install libvirt and rebuild with: go build -tags libvirt")
		},
	}
}
