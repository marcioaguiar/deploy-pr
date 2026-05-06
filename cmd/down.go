package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

func newDownCmd(root *rootOpts) *cobra.Command {
	var (
		commentOnPR   bool
		keepNamespace bool
	)
	cmd := &cobra.Command{
		Use:   "down <PR#>",
		Short: "Tear down the preview environment for the given PR",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, _ []string) error {
			_ = root
			_ = commentOnPR
			_ = keepNamespace
			return errors.New("down: not yet implemented (U5)")
		},
	}
	cmd.Flags().BoolVar(&commentOnPR, "comment-on-pr", false, "update the sticky preview comment to indicate teardown")
	cmd.Flags().BoolVar(&keepNamespace, "keep-namespace", false, "uninstall the Helm release but keep the namespace")
	return cmd
}
