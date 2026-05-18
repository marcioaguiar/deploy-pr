// Package git fetches a GitHub PR's head tree into a local directory so that
// `deploy-pr up --clone` can run from anywhere — not just from a checkout of
// the target repo. Only the single commit is fetched (--depth 1) and the PR
// is referenced via refs/pull/<N>/head, which works uniformly for same-repo
// and fork PRs because GitHub exposes that ref on the base repo.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Runner abstracts process invocation so tests can replace it with a recorder
// without spawning real subprocesses. Mirrors docker.Runner.
type Runner interface {
	LookPath(name string) error
	Run(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error
}

// FetchOptions describes a single shallow PR-head fetch.
type FetchOptions struct {
	Owner    string
	Repo     string
	PRNumber int
	// Token is embedded in the clone URL as x-access-token. Empty means
	// anonymous, which only works for public repos.
	Token string
	// Runner is optional; nil means use the real git CLI.
	Runner Runner
}

// ShallowFetch initialises a fresh repo in a temp directory, fetches
// refs/pull/<N>/head at depth 1, and checks out FETCH_HEAD. It returns the
// directory path and a cleanup function the caller must invoke when done
// (typically via defer).
//
// The cleanup is also called automatically on error before returning.
func ShallowFetch(ctx context.Context, opts FetchOptions) (string, func(), error) {
	if opts.Owner == "" || opts.Repo == "" {
		return "", nil, errors.New("git: owner and repo are required")
	}
	if opts.PRNumber <= 0 {
		return "", nil, errors.New("git: PR number must be positive")
	}
	runner := opts.Runner
	if runner == nil {
		runner = &execRunner{}
	}
	if err := runner.LookPath("git"); err != nil {
		return "", nil, fmt.Errorf("git CLI not on PATH (install git or run from CI): %w", err)
	}

	dir, err := os.MkdirTemp("", fmt.Sprintf("deploy-pr-%d-", opts.PRNumber))
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }

	url := cloneURL(opts.Owner, opts.Repo, opts.Token)
	ref := fmt.Sprintf("refs/pull/%d/head", opts.PRNumber)

	steps := [][]string{
		{"init", "--quiet", dir},
		{"-C", dir, "remote", "add", "origin", url},
		{"-C", dir, "fetch", "--depth", "1", "--quiet", "origin", ref},
		{"-C", dir, "checkout", "--quiet", "FETCH_HEAD"},
	}

	for _, args := range steps {
		var stderr bytes.Buffer
		if err := runner.Run(ctx, "git", args, io.Discard, &stderr); err != nil {
			cleanup()
			msg := scrub(stderr.String(), opts.Token)
			return "", nil, fmt.Errorf("git %s failed: %w: %s", args[0], err, strings.TrimSpace(msg))
		}
	}

	return dir, cleanup, nil
}

// cloneURL builds a token-authenticated HTTPS URL when a token is present;
// anonymous HTTPS otherwise. Token is never logged by this package.
func cloneURL(owner, repo, token string) string {
	if token != "" {
		return fmt.Sprintf("https://x-access-token:%s@github.com/%s/%s.git", token, owner, repo)
	}
	return fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)
}

// scrub removes the token from a string so it never reaches user-visible
// errors or logs.
func scrub(s, token string) string {
	if token == "" {
		return s
	}
	return strings.ReplaceAll(s, token, "***")
}

type execRunner struct{}

func (e *execRunner) LookPath(name string) error {
	_, err := exec.LookPath(name)
	return err
}

func (e *execRunner) Run(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}
