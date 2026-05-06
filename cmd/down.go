package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/marcioaguiar/deploy-pr/internal/helm"
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
		RunE: func(c *cobra.Command, args []string) error {
			number, err := strconv.Atoi(args[0])
			if err != nil || number <= 0 {
				return userErr(fmt.Errorf("PR number must be a positive integer, got %q", args[0]))
			}

			namespace := fmt.Sprintf("%s%d", root.cfg.Namespace.Prefix, number)

			mgr, err := helm.NewManager(namespace, root.logger)
			if err != nil {
				return infraErr(err)
			}

			if err := mgr.Uninstall(namespace); err != nil {
				return infraErr(err)
			}
			root.logger.Info("helm release uninstalled", "release", namespace)

			if !keepNamespace {
				if err := mgr.DeleteNamespace(c.Context(), namespace); err != nil {
					return infraErr(err)
				}
				root.logger.Info("namespace deleted", "namespace", namespace)
			}

			fmt.Fprintf(root.stdout, "Preview pr-%d torn down\n", number)
			_ = commentOnPR // U6 wires sticky-comment update here.
			return nil
		},
	}
	cmd.Flags().BoolVar(&commentOnPR, "comment-on-pr", false, "update the sticky preview comment to indicate teardown")
	cmd.Flags().BoolVar(&keepNamespace, "keep-namespace", false, "uninstall the Helm release but keep the namespace")
	return cmd
}
