package cmd

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/marcioaguiar/deploy-pr/internal/docker"
	"github.com/marcioaguiar/deploy-pr/internal/ecr"
	gitpkg "github.com/marcioaguiar/deploy-pr/internal/git"
	ghpkg "github.com/marcioaguiar/deploy-pr/internal/github"
	"github.com/marcioaguiar/deploy-pr/internal/helm"
)

func newUpCmd(root *rootOpts) *cobra.Command {
	var (
		commentOnPR     bool
		dryRun          bool
		imageTag        string
		clone           bool
		createNamespace bool
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

			namespace := fmt.Sprintf("%s%d", root.cfg.Namespace.Prefix, pr.Number)
			host := fmt.Sprintf("%s.%s", namespace, root.cfg.BaseDomain)

			if dryRun {
				fmt.Fprintf(root.stdout, "PR #%d on %s/%s\n", pr.Number, pr.RepoOwner, pr.RepoName)
				fmt.Fprintf(root.stdout, "  title:     %s\n", pr.Title)
				fmt.Fprintf(root.stdout, "  author:    %s\n", pr.Author)
				fmt.Fprintf(root.stdout, "  branch:    %s -> %s\n", pr.HeadRef, pr.BaseRef)
				fmt.Fprintf(root.stdout, "  sha:       %s\n", pr.HeadSHA)
				fmt.Fprintf(root.stdout, "  draft:     %t\n", pr.IsDraft)
				fmt.Fprintf(root.stdout, "  namespace: %s\n", namespace)
				if root.cfg.BaseDomain != "" {
					fmt.Fprintf(root.stdout, "  url:       https://%s\n", host)
				}
				if root.cfg.ECRRepoURI != "" {
					tag := imageTag
					if tag == "" {
						tag = fmt.Sprintf("pr-%d-%s", pr.Number, pr.ShortSHA())
					}
					fmt.Fprintf(root.stdout, "  image:     %s:%s\n", root.cfg.ECRRepoURI, tag)
				}
				fmt.Fprintln(root.stdout, "(dry-run: no build, push, or deploy performed)")
				return nil
			}

			if err := root.cfg.ValidateForUp(); err != nil {
				return userErr(err)
			}

			tag := imageTag
			if tag == "" {
				tag = fmt.Sprintf("pr-%d-%s", pr.Number, pr.ShortSHA())
				contextDir := "."
				if clone {
					dir, cleanup, err := clonePRSource(c.Context(), root, pr)
					if err != nil {
						return err
					}
					defer cleanup()
					contextDir = dir
				}
				if err := buildAndPushImage(c.Context(), root, pr, tag, contextDir); err != nil {
					return err
				}
			} else {
				root.logger.Info("skipping build, using --image-tag override", "tag", tag)
			}

			if err := installPreview(c.Context(), root, pr, tag, namespace, host, createNamespace); err != nil {
				return err
			}

			fmt.Fprintf(root.stdout, "Preview ready: https://%s\n", host)

			if commentOnPR {
				postReadyComment(c.Context(), root, pr, tag, fmt.Sprintf("https://%s", host))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&commentOnPR, "comment-on-pr", false, "post or update a sticky preview-URL comment on the PR")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "resolve PR and report what would happen, then exit")
	cmd.Flags().StringVar(&imageTag, "image-tag", "", "use a pre-built image tag instead of building locally")
	cmd.Flags().BoolVar(&clone, "clone", false, "shallow-fetch the PR head into a temp dir and build from there (default: build from current directory)")
	cmd.Flags().BoolVar(&createNamespace, "create-namespace", true, "create the target namespace if missing (requires cluster-scoped 'create namespaces' RBAC; set false when an admin pre-creates pr-<N>)")
	return cmd
}

func buildAndPushImage(ctx context.Context, root *rootOpts, pr *ghpkg.PRInfo, tag, contextDir string) error {
	root.logger.Info("authenticating to ECR", "region", root.cfg.Region)
	auth, err := ecr.New(ctx, root.cfg.Region)
	if err != nil {
		return infraErr(fmt.Errorf("ECR auth init: %w", err))
	}
	if _, err := auth.LoginToECR(ctx); err != nil {
		return infraErr(err)
	}

	latest := fmt.Sprintf("pr-%d-latest", pr.Number)
	root.logger.Info("building image",
		"tags", []string{tag, latest},
		"context", contextDir,
	)
	builder := docker.New(root.stderr, root.stderr)
	return mapBuildErr(builder.Build(ctx, docker.BuildOptions{
		ContextDir: contextDir,
		Tags: []string{
			fmt.Sprintf("%s:%s", root.cfg.ECRRepoURI, tag),
			fmt.Sprintf("%s:%s", root.cfg.ECRRepoURI, latest),
		},
		Push: true,
	}))
}

// clonePRSource shallow-fetches the PR's head tree into a temp directory and
// returns the path along with a cleanup function. The returned cleanup must
// be called by the caller (typically via defer) once the build context is no
// longer needed.
func clonePRSource(ctx context.Context, root *rootOpts, pr *ghpkg.PRInfo) (string, func(), error) {
	token, source, err := ghpkg.TokenResolver{Explicit: root.githubToken}.Resolve()
	if err != nil {
		return "", nil, userErr(fmt.Errorf("--clone needs a GitHub token: %w", err))
	}
	root.logger.Info("cloning PR source",
		"repo", fmt.Sprintf("%s/%s", pr.RepoOwner, pr.RepoName),
		"pr", pr.Number,
		"token_source", string(source),
	)
	dir, cleanup, err := gitpkg.ShallowFetch(ctx, gitpkg.FetchOptions{
		Owner:    pr.RepoOwner,
		Repo:     pr.RepoName,
		PRNumber: pr.Number,
		Token:    token,
	})
	if err != nil {
		return "", nil, infraErr(err)
	}
	root.logger.Info("cloned PR source", "dir", dir)
	return dir, cleanup, nil
}

func mapBuildErr(err error) error {
	if err == nil {
		return nil
	}
	return infraErr(err)
}

// postReadyComment posts (or updates) the sticky preview comment. Failures
// are logged but do not fail `up` -- the deploy already succeeded and a
// missing comment is recoverable by re-running.
func postReadyComment(ctx context.Context, root *rootOpts, pr *ghpkg.PRInfo, tag, url string) {
	token, source, err := ghpkg.TokenResolver{Explicit: root.githubToken}.Resolve()
	if err != nil {
		root.logger.Warn("comment-on-pr: no token available, skipping", "err", err)
		return
	}
	client := ghpkg.NewClient(token, source)
	body := ghpkg.CommentInput{
		Status:    ghpkg.StatusReady,
		URL:       url,
		ImageTag:  fmt.Sprintf("%s:%s", root.cfg.ECRRepoURI, tag),
		Timestamp: time.Now(),
	}.RenderBody()
	id, err := client.UpsertStickyComment(ctx, pr.RepoOwner, pr.RepoName, pr.Number, body)
	if err != nil {
		root.logger.Warn("comment-on-pr: failed to post sticky comment (deploy succeeded)", "err", err)
		return
	}
	root.logger.Info("comment-on-pr: sticky comment posted", "id", id)
}

func installPreview(ctx context.Context, root *rootOpts, pr *ghpkg.PRInfo, tag, namespace, host string, createNamespace bool) error {
	chart, err := helm.LoadChart(root.cfg.ChartPath)
	if err != nil {
		return userErr(fmt.Errorf("load chart %s: %w", root.cfg.ChartPath, err))
	}

	values := helm.BuildValues(helm.ValuesInput{
		PRNumber:        pr.Number,
		ShortSHA:        pr.ShortSHA(),
		ImageRepository: root.cfg.ECRRepoURI,
		ImageTag:        tag,
		Host:            host,
	})

	mgr, err := helm.NewManager(namespace, root.logger)
	if err != nil {
		return infraErr(err)
	}

	rel, err := mgr.UpgradeOrInstall(ctx, helm.UpgradeOptions{
		ReleaseName:     namespace,
		Namespace:       namespace,
		Chart:           chart,
		Values:          values,
		Timeout:         root.cfg.Timeout,
		CreateNamespace: createNamespace,
	})
	if err != nil {
		return infraErr(fmt.Errorf("helm upgrade --install: %w", err))
	}
	root.logger.Info("helm release ready",
		"release", rel.Name,
		"namespace", rel.Namespace,
		"revision", rel.Version,
		"status", rel.Info.Status.String(),
	)
	return nil
}
