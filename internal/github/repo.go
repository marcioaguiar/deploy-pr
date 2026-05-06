package github

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// ErrCannotDetectRepo means the CLI is not running inside a git checkout, the
// remote isn't named "origin", or its URL doesn't look like GitHub.
var ErrCannotDetectRepo = errors.New("cannot detect GitHub repo from git remote")

// sshRemote matches  git@github.com:owner/name(.git)?
var sshRemote = regexp.MustCompile(`^git@github\.com:([^/]+)/([^/]+?)(?:\.git)?$`)

// httpsRemote matches https://github.com/owner/name(.git)? with optional
// userinfo (e.g. https://x-access-token:T@github.com/owner/name.git as used
// in CI runners).
var httpsRemote = regexp.MustCompile(`^https://(?:[^@]+@)?github\.com/([^/]+)/([^/]+?)(?:\.git)?$`)

// ParseRemoteURL extracts owner/name from a GitHub remote URL. It accepts
// SSH and HTTPS forms, with or without a trailing .git.
func ParseRemoteURL(raw string) (owner, name string, err error) {
	raw = strings.TrimSpace(raw)
	if m := sshRemote.FindStringSubmatch(raw); m != nil {
		return m[1], m[2], nil
	}
	if m := httpsRemote.FindStringSubmatch(raw); m != nil {
		return m[1], m[2], nil
	}
	return "", "", fmt.Errorf("%w: %q is not a recognised GitHub URL", ErrCannotDetectRepo, raw)
}

// DetectRepoFromGit shells out to `git remote get-url origin` and parses the
// result. Replaceable in tests via the gitRemoteURL package variable.
func DetectRepoFromGit() (owner, name string, err error) {
	url, err := gitRemoteURL("origin")
	if err != nil {
		return "", "", fmt.Errorf("%w: %s", ErrCannotDetectRepo, err)
	}
	return ParseRemoteURL(url)
}

// gitRemoteURL is overridable in tests.
var gitRemoteURL = func(remote string) (string, error) {
	out, err := exec.Command("git", "remote", "get-url", remote).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", fmt.Errorf("git remote get-url %s: %s", remote, strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
