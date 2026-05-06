package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// These are populated via -ldflags at build time. GoReleaser (U9) sets them.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func newVersionCmd(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(_ *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(out, "deploy-pr %s (commit %s, built %s)\n", version, commit, date)
			return err
		},
	}
}
