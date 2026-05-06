package cmd

import (
	"context"
	"errors"
	"fmt"

	ghpkg "github.com/marcioaguiar/deploy-pr/internal/github"
)

// resolvePR fetches PR metadata using whichever repo and token source the
// rootOpts resolves. Returns userErr for things the user controls (bad PR
// number, missing token, repo detection failed) and infraErr for transient
// GitHub failures.
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
		case errors.Is(err, ghpkg.ErrPRNotFound), errors.Is(err, ghpkg.ErrAuthFailed):
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
	if root.cfg.Repo == "" {
		o, n, err := ghpkg.DetectRepoFromGit()
		if err != nil {
			return "", "", fmt.Errorf("%w (set --repo or repo: in config to override)", err)
		}
		return o, n, nil
	}
	o, n, ok := splitOwnerName(root.cfg.Repo)
	if !ok {
		return "", "", fmt.Errorf("repo %q must be in owner/name form", root.cfg.Repo)
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
