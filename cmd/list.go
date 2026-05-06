package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

func newListCmd(root *rootOpts) *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List active preview environments",
		RunE: func(_ *cobra.Command, _ []string) error {
			_ = root
			_ = output
			return errors.New("list: not yet implemented (U7)")
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "text", "output format: text or json")
	return cmd
}
