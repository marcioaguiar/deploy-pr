package git

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

type recordedCall struct {
	name string
	args []string
}

type fakeRunner struct {
	lookPathErr error
	runErr      error
	runErrAt    int // 0 = never; 1 = first call, etc.

	calls []recordedCall
	// optional stderr to write on the failing call
	stderrOnFail string
}

func (f *fakeRunner) LookPath(string) error { return f.lookPathErr }

func (f *fakeRunner) Run(_ context.Context, name string, args []string, _, stderr io.Writer) error {
	f.calls = append(f.calls, recordedCall{name: name, args: append([]string(nil), args...)})
	if f.runErrAt != 0 && len(f.calls) == f.runErrAt {
		if stderr != nil && f.stderrOnFail != "" {
			_, _ = stderr.Write([]byte(f.stderrOnFail))
		}
		return f.runErr
	}
	return nil
}

func TestShallowFetch_RunsExpectedSteps(t *testing.T) {
	r := &fakeRunner{}
	dir, cleanup, err := ShallowFetch(context.Background(), FetchOptions{
		Owner:    "edgebr",
		Repo:     "performance-rastreamento",
		PRNumber: 3075,
		Token:    "ghp_secret",
		Runner:   r,
	})
	if err != nil {
		t.Fatalf("ShallowFetch: %v", err)
	}
	defer cleanup()

	if dir == "" {
		t.Fatal("expected non-empty dir")
	}
	if _, statErr := os.Stat(dir); statErr != nil {
		t.Fatalf("temp dir not created: %v", statErr)
	}
	if len(r.calls) != 4 {
		t.Fatalf("expected 4 git calls, got %d: %+v", len(r.calls), r.calls)
	}

	// init
	if got := r.calls[0].args; got[0] != "init" || got[len(got)-1] != dir {
		t.Errorf("init args = %v", got)
	}
	// remote add origin <url-with-token>
	remoteArgs := r.calls[1].args
	if remoteArgs[2] != "remote" || remoteArgs[3] != "add" || remoteArgs[4] != "origin" {
		t.Errorf("remote add args = %v", remoteArgs)
	}
	url := remoteArgs[5]
	if !strings.Contains(url, "x-access-token:ghp_secret@github.com/edgebr/performance-rastreamento.git") {
		t.Errorf("clone URL not authenticated: %q", url)
	}
	// fetch --depth 1 origin refs/pull/3075/head
	fetchArgs := r.calls[2].args
	wantFetchTail := []string{"fetch", "--depth", "1", "--quiet", "origin", "refs/pull/3075/head"}
	if !endsWith(fetchArgs, wantFetchTail) {
		t.Errorf("fetch args = %v, want tail %v", fetchArgs, wantFetchTail)
	}
	// checkout FETCH_HEAD
	checkoutArgs := r.calls[3].args
	if checkoutArgs[len(checkoutArgs)-1] != "FETCH_HEAD" {
		t.Errorf("checkout args = %v", checkoutArgs)
	}
}

func TestShallowFetch_AnonymousURLWhenNoToken(t *testing.T) {
	r := &fakeRunner{}
	_, cleanup, err := ShallowFetch(context.Background(), FetchOptions{
		Owner: "edgebr", Repo: "x", PRNumber: 1, Runner: r,
	})
	if err != nil {
		t.Fatalf("ShallowFetch: %v", err)
	}
	defer cleanup()

	url := r.calls[1].args[5]
	if strings.Contains(url, "x-access-token") {
		t.Errorf("anonymous URL should not embed token, got %q", url)
	}
	if !strings.HasPrefix(url, "https://github.com/edgebr/x.git") {
		t.Errorf("unexpected anonymous URL: %q", url)
	}
}

func TestShallowFetch_ScrubsTokenFromError(t *testing.T) {
	r := &fakeRunner{
		runErr:       errors.New("exit 128"),
		runErrAt:     3, // fail on fetch
		stderrOnFail: "fatal: could not authenticate to https://x-access-token:ghp_secret@github.com/edgebr/x.git",
	}
	_, _, err := ShallowFetch(context.Background(), FetchOptions{
		Owner: "edgebr", Repo: "x", PRNumber: 1, Token: "ghp_secret", Runner: r,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "ghp_secret") {
		t.Errorf("error must not leak token: %v", err)
	}
	if !strings.Contains(err.Error(), "***") {
		t.Errorf("expected scrubbed marker in error: %v", err)
	}
}

func TestShallowFetch_CleansUpOnFailure(t *testing.T) {
	r := &fakeRunner{runErr: errors.New("exit 1"), runErrAt: 3}
	_, _, err := ShallowFetch(context.Background(), FetchOptions{
		Owner: "edgebr", Repo: "x", PRNumber: 1, Token: "t", Runner: r,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	// The temp dir name we can't easily recover here without exposing it,
	// but we can check that no deploy-pr-* dirs were left behind under
	// os.TempDir for this PR number. We use a marker token in the prefix.
	entries, _ := os.ReadDir(os.TempDir())
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "deploy-pr-1-") {
			full := os.TempDir() + string(os.PathSeparator) + e.Name()
			if _, statErr := os.Stat(full); statErr == nil {
				_ = os.RemoveAll(full)
				t.Errorf("temp dir %s leaked after failure", full)
			}
		}
	}
}

func TestShallowFetch_ValidatesArgs(t *testing.T) {
	cases := []struct {
		name string
		opts FetchOptions
	}{
		{"missing owner", FetchOptions{Repo: "x", PRNumber: 1}},
		{"missing repo", FetchOptions{Owner: "x", PRNumber: 1}},
		{"zero PR", FetchOptions{Owner: "x", Repo: "y", PRNumber: 0}},
		{"negative PR", FetchOptions{Owner: "x", Repo: "y", PRNumber: -3}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := ShallowFetch(context.Background(), tc.opts)
			if err == nil {
				t.Errorf("expected validation error for %v", tc.opts)
			}
		})
	}
}

func TestShallowFetch_GitNotOnPath(t *testing.T) {
	r := &fakeRunner{lookPathErr: errors.New("not found")}
	_, _, err := ShallowFetch(context.Background(), FetchOptions{
		Owner: "x", Repo: "y", PRNumber: 1, Runner: r,
	})
	if err == nil || !strings.Contains(err.Error(), "git CLI not on PATH") {
		t.Errorf("expected friendly missing-git error, got %v", err)
	}
}

func endsWith(s, tail []string) bool {
	if len(tail) > len(s) {
		return false
	}
	for i := range tail {
		if s[len(s)-len(tail)+i] != tail[i] {
			return false
		}
	}
	return true
}
