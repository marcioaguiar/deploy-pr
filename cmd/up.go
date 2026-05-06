package cmd

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	ghpkg "github.com/marcioaguiar/deploy-pr/internal/github"
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
		RunE: func(c *cobra.Command, args []string) error {
			number, err := strconv.Atoi(args[0])
			if err != nil || number <= 0 {
				return userErr(fmt.Errorf("PR number must be a positive integer, got %q", args[0]))
			}

			pr, err := resolvePR(c.Context(), root, number)
			if err != nil {
				return err
			}

			root.logger.Info("resolved PR",
				"pr", pr.Number,
				"repo", fmt.Sprintf("%s/%s", pr.RepoOwner, pr.RepoName),
				"head_sha", pr.ShortSHA(),
				"head_ref", pr.HeadRef,
				"author", pr.Author,
				"draft", pr.IsDraft,
			)

			if dryRun {
				fmt.Fprintf(root.stdout, "PR #%d on %s/%s\n", pr.Number, pr.RepoOwner, pr.RepoName)
				fmt.Fprintf(root.stdout, "  title:   %s\n", pr.Title)
				fmt.Fprintf(root.stdout, "  author:  %s\n", pr.Author)
				fmt.Fprintf(root.stdout, "  branch:  %s -> %s\n", pr.HeadRef, pr.BaseRef)
				fmt.Fprintf(root.stdout, "  sha:     %s\n", pr.HeadSHA)
				fmt.Fprintf(root.stdout, "  draft:   %t\n", pr.IsDraft)
				if root.cfg.BaseDomain != "" {
					fmt.Fprintf(root.stdout, "  url:     https://%s%d.%s\n",
						root.cfg.Namespace.Prefix, pr.Number, root.cfg.BaseDomain)
				}
				fmt.Fprintln(root.stdout, "(dry-run: no build, push, or deploy performed)")
				return nil
			}

			_ = commentOnPR
			_ = imageTag
			return errors.New("up: build/push/deploy not yet implemented (U3-U5)")
		},
	}
	cmd.Flags().BoolVar(&commentOnPR, "comment-on-pr", false, "post or update a sticky preview-URL comment on the PR")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "resolve PR and report what would happen, then exit")
	cmd.Flags().StringVar(&imageTag, "image-tag", "", "use a pre-built image tag instead of building locally")
	return cmd
}

// resolvePR is shared by up and down: it determines the target repo, resolves
// a token, and fetches PR metadata.
func resolvePR(ctx context.Context, root *rootOpts, number int) (*ghpkg.PRInfo, error) {
	owner, name, err := repoFor(root)
	if err != nil {
		return nil, userErr(err)
	}

	token, source, err := ghpkg.TokenResolver{Explicit: root.githubToken}.Resolve()
	if err != nil {
		return nil, userErr(err)
	}
	root.logger.Debug("resolved github token", "source", source)

	client := ghpkg.NewClient(token, source)
	pr, err := client.GetPR(ctx, owner, name, number)
	if err != nil {
		switch {
		case errors.Is(err, ghpkg.ErrPRNotFound):
			return nil, userErr(err)
		case errors.Is(err, ghpkg.ErrAuthFailed):
			return nil, userErr(err)
		default:
			return nil, infraErr(err)
		}
	}
	return pr, nil
}

// repoFor returns owner, name based on --repo, config.repo, or git remote
// auto-detection -- in that order.
func repoFor(root *rootOpts) (string, string, error) {
	configured := root.cfg.Repo
	if configured == "" {
		o, n, err := ghpkg.DetectRepoFromGit()
		if err != nil {
			return "", "", fmt.Errorf("%w (set --repo or repo: in config to override)", err)
		}
		return o, n, nil
	}
	o, n, ok := splitOwnerName(configured)
	if !ok {
		return "", "", fmt.Errorf("repo %q must be in owner/name form", configured)
	}
	return o, n, nil
}

func splitOwnerName(s string) (string, string, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			if i == 0 || i == len(s)-1 {
				return "", "", false
			}
			return s[:i], s[i+1:], true
		}
	}
	return "", "", false
}
