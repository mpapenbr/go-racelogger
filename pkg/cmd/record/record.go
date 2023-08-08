package record

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewRecordCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "record",
		Short: "record an iRacing event",
		RunE: func(cmd *cobra.Command, args []string) error {
			return recordEvent()
		},
	}

	return cmd
}

func recordEvent() error {
	fmt.Println("not yet implemented")
	return nil
}
