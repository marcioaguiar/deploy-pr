package github

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// TokenSource describes where a successfully resolved GitHub token came from.
// The string is surfaced in auth-failure errors so users know which knob to
// adjust.
type TokenSource string

const (
	SourceFlag TokenSource = "flag"
	SourceEnv  TokenSource = "env GITHUB_TOKEN"
	SourceGH   TokenSource = "gh CLI"
)

// ErrNoToken means none of the configured sources yielded a GitHub token.
var ErrNoToken = errors.New("no GitHub token available")

// TokenResolver resolves a GitHub token from explicit flag, env, or `gh`.
// Precedence: flag > env > gh. Fields with function types are exposed so
// tests can stub them; nil values mean "use the real implementation".
type TokenResolver struct {
	Explicit string

	// Env returns the value of an environment variable. Defaults to os.Getenv.
	Env func(string) string
	// GHToken shells out to `gh auth token`. Defaults to ghCLIToken.
	GHToken func() (string, error)
}

// Resolve returns the token and the source that produced it. Returns ErrNoToken
// (wrapped) if every source is empty or unavailable.
func (r TokenResolver) Resolve() (string, TokenSource, error) {
	if r.Explicit != "" {
		return r.Explicit, SourceFlag, nil
	}

	getenv := r.Env
	if getenv == nil {
		getenv = os.Getenv
	}
	if t := strings.TrimSpace(getenv("GITHUB_TOKEN")); t != "" {
		return t, SourceEnv, nil
	}

	gh := r.GHToken
	if gh == nil {
		gh = ghCLIToken
	}
	if t, err := gh(); err == nil && t != "" {
		return t, SourceGH, nil
	}

	return "", "", fmt.Errorf("%w: tried --github-token, GITHUB_TOKEN env, and `gh auth token`", ErrNoToken)
}

// ghCLIToken runs `gh auth token` and returns the printed token. Returns an
// error if `gh` is not on PATH or auth is not configured.
func ghCLIToken() (string, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return "", fmt.Errorf("gh CLI not on PATH: %w", err)
	}
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", fmt.Errorf("gh auth token failed: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
