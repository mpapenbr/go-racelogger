package ping

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewPingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ping",
		Short: "check connection to backend server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return pingBackend()
		},
	}

	return cmd
}

func pingBackend() error {
	fmt.Println("not yet implemented")
	return nil
}
