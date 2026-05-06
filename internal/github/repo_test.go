package github

import (
	"errors"
	"strings"
	"testing"
)

func TestParseRemoteURL(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		owner     string
		repo      string
		expectErr bool
	}{
		{"ssh", "git@github.com:foo/bar.git", "foo", "bar", false},
		{"ssh no .git", "git@github.com:foo/bar", "foo", "bar", false},
		{"https", "https://github.com/foo/bar.git", "foo", "bar", false},
		{"https no .git", "https://github.com/foo/bar", "foo", "bar", false},
		{"https with userinfo", "https://x-access-token:abc123@github.com/foo/bar.git", "foo", "bar", false},
		{"trailing whitespace", "  git@github.com:foo/bar.git\n", "foo", "bar", false},
		{"hyphen in name", "git@github.com:my-org/my-repo.git", "my-org", "my-repo", false},
		{"non-github", "git@gitlab.com:foo/bar.git", "", "", true},
		{"garbage", "not a url", "", "", true},
		{"empty", "", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			owner, name, err := ParseRemoteURL(tc.input)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error for %q, got %s/%s", tc.input, owner, name)
				}
				if !errors.Is(err, ErrCannotDetectRepo) {
					t.Errorf("error %v does not wrap ErrCannotDetectRepo", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
			if owner != tc.owner || name != tc.repo {
				t.Errorf("got %s/%s, want %s/%s", owner, name, tc.owner, tc.repo)
			}
		})
	}
}

func TestDetectRepoFromGit_StubbedRemote(t *testing.T) {
	prev := gitRemoteURL
	t.Cleanup(func() { gitRemoteURL = prev })

	gitRemoteURL = func(remote string) (string, error) {
		if remote != "origin" {
			t.Fatalf("expected remote=origin, got %q", remote)
		}
		return "https://github.com/foo/bar.git", nil
	}

	owner, name, err := DetectRepoFromGit()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if owner != "foo" || name != "bar" {
		t.Errorf("got %s/%s", owner, name)
	}
}

func TestDetectRepoFromGit_GitFails(t *testing.T) {
	prev := gitRemoteURL
	t.Cleanup(func() { gitRemoteURL = prev })

	gitRemoteURL = func(string) (string, error) {
		return "", errors.New("not a git repo")
	}

	_, _, err := DetectRepoFromGit()
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrCannotDetectRepo) {
		t.Errorf("expected ErrCannotDetectRepo, got %v", err)
	}
	if !strings.Contains(err.Error(), "not a git repo") {
		t.Errorf("error should include underlying message: %v", err)
	}
}
