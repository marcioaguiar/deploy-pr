package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

func newUpCmd(root *rootOpts) *cobra.Command {
	var (
		commentOnPR bool
		dryRun      bool
		imageTag    string
	)
	cmd := &cobra.Command{
		Use:   "up <PR#>",
		Short: "Build, push, and deploy a preview environment for the given PR",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, _ []string) error {
			_ = root
			_ = commentOnPR
			_ = dryRun
			_ = imageTag
			return errors.New("up: not yet implemented (U5)")
		},
	}
	cmd.Flags().BoolVar(&commentOnPR, "comment-on-pr", false, "post or update a sticky preview-URL comment on the PR")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "resolve PR and report what would happen, then exit")
	cmd.Flags().StringVar(&imageTag, "image-tag", "", "use a pre-built image tag instead of building locally")
	return cmd
}
