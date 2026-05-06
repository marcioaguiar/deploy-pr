package cmd

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	ghpkg "github.com/marcioaguiar/deploy-pr/internal/github"
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

			if commentOnPR {
				postTorndownComment(c.Context(), root, number)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&commentOnPR, "comment-on-pr", false, "update the sticky preview comment to indicate teardown")
	cmd.Flags().BoolVar(&keepNamespace, "keep-namespace", false, "uninstall the Helm release but keep the namespace")
	return cmd
}

// postTorndownComment marks the sticky comment as torn down. Like the up
// path, failures are logged but do not fail the command.
func postTorndownComment(ctx context.Context, root *rootOpts, prNumber int) {
	owner, name, err := repoFor(root)
	if err != nil {
		root.logger.Warn("comment-on-pr: cannot determine repo, skipping", "err", err)
		return
	}
	token, source, err := ghpkg.TokenResolver{Explicit: root.githubToken}.Resolve()
	if err != nil {
		root.logger.Warn("comment-on-pr: no token available, skipping", "err", err)
		return
	}
	client := ghpkg.NewClient(token, source)
	url := ""
	if root.cfg.BaseDomain != "" {
		url = fmt.Sprintf("https://%s%d.%s", root.cfg.Namespace.Prefix, prNumber, root.cfg.BaseDomain)
	}
	body := ghpkg.CommentInput{
		Status:    ghpkg.StatusTornDown,
		URL:       url,
		Timestamp: time.Now(),
	}.RenderBody()
	id, err := client.UpsertStickyComment(ctx, owner, name, prNumber, body)
	if err != nil {
		root.logger.Warn("comment-on-pr: failed to update sticky comment (teardown succeeded)", "err", err)
		return
	}
	root.logger.Info("comment-on-pr: sticky comment marked torn-down", "id", id)
}
