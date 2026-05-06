package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestClient stands up an httptest server and points a Client at it.
// handler receives the inner mux so each test can register its own routes.
func newTestClient(t *testing.T, source TokenSource, handler func(*http.ServeMux)) (*Client, *httptest.Server) {
	t.Helper()
	mux := http.NewServeMux()
	if handler != nil {
		handler(mux)
	}
	// go-github's enterprise base URL must end with a trailing slash and is
	// expected to host the v3 API rooted at /api/v3/. We serve / instead and
	// just make sure the registered routes match what go-github calls.
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c, err := newClientWithBaseURL("test-token", source, srv.URL+"/")
	if err != nil {
		t.Fatalf("newClientWithBaseURL: %v", err)
	}
	return c, srv
}

func TestGetPR_HappyPath(t *testing.T) {
	c, _ := newTestClient(t, SourceEnv, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/v3/repos/octo/widgets/pulls/42", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("unexpected method: %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{
				"number": 42,
				"title": "Add widget",
				"draft": false,
				"html_url": "https://github.com/octo/widgets/pull/42",
				"head": {"sha": "abc1234567890", "ref": "feat/widget"},
				"base": {"ref": "main"},
				"user": {"login": "alice"}
			}`)
		})
	})

	info, err := c.GetPR(context.Background(), "octo", "widgets", 42)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if info.Number != 42 {
		t.Errorf("Number = %d", info.Number)
	}
	if info.HeadSHA != "abc1234567890" {
		t.Errorf("HeadSHA = %q", info.HeadSHA)
	}
	if info.ShortSHA() != "abc1234" {
		t.Errorf("ShortSHA = %q", info.ShortSHA())
	}
	if info.HeadRef != "feat/widget" || info.BaseRef != "main" {
		t.Errorf("refs = %s -> %s", info.HeadRef, info.BaseRef)
	}
	if info.Author != "alice" {
		t.Errorf("Author = %q", info.Author)
	}
	if info.IsDraft {
		t.Errorf("expected non-draft")
	}
	if info.Title != "Add widget" {
		t.Errorf("Title = %q", info.Title)
	}
}

func TestGetPR_DraftReportedNotBlocked(t *testing.T) {
	c, _ := newTestClient(t, SourceEnv, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/v3/repos/octo/widgets/pulls/7", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{
				"number": 7,
				"draft": true,
				"head": {"sha": "deadbeefdead", "ref": "wip"},
				"base": {"ref": "main"},
				"user": {"login": "alice"}
			}`)
		})
	})

	info, err := c.GetPR(context.Background(), "octo", "widgets", 7)
	if err != nil {
		t.Fatalf("expected no error for draft PR: %v", err)
	}
	if !info.IsDraft {
		t.Error("expected IsDraft=true")
	}
}

func TestGetPR_404IsErrPRNotFound(t *testing.T) {
	c, _ := newTestClient(t, SourceEnv, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/v3/repos/octo/widgets/pulls/999", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		})
	})

	_, err := c.GetPR(context.Background(), "octo", "widgets", 999)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrPRNotFound) {
		t.Errorf("expected ErrPRNotFound, got %v", err)
	}
	if !strings.Contains(err.Error(), "octo/widgets#999") {
		t.Errorf("error should name the PR: %v", err)
	}
}

func TestGetPR_401NamesTokenSource(t *testing.T) {
	c, _ := newTestClient(t, SourceGH, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/v3/repos/octo/widgets/pulls/1", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
		})
	})

	_, err := c.GetPR(context.Background(), "octo", "widgets", 1)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrAuthFailed) {
		t.Errorf("expected ErrAuthFailed, got %v", err)
	}
	if !strings.Contains(err.Error(), string(SourceGH)) {
		t.Errorf("error should name token source %q: %v", SourceGH, err)
	}
}

func TestGetPR_RejectsBadInput(t *testing.T) {
	c := NewClient("x", SourceFlag)
	if _, err := c.GetPR(context.Background(), "", "x", 1); err == nil {
		t.Error("expected error for empty owner")
	}
	if _, err := c.GetPR(context.Background(), "x", "x", 0); err == nil {
		t.Error("expected error for zero PR number")
	}
}

func TestGetPR_MissingHeadSHA(t *testing.T) {
	c, _ := newTestClient(t, SourceEnv, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/v3/repos/octo/widgets/pulls/3", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			// head.sha intentionally omitted -- simulates a PR whose branch was deleted.
			fmt.Fprintln(w, `{"number": 3, "head": {"ref": "gone"}, "base": {"ref": "main"}, "user": {"login": "alice"}}`)
		})
	})

	_, err := c.GetPR(context.Background(), "octo", "widgets", 3)
	if err == nil {
		t.Fatal("expected error for empty head SHA")
	}
	if !strings.Contains(err.Error(), "head SHA") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPRInfo_ShortSHA(t *testing.T) {
	cases := map[string]string{
		"abc1234567890": "abc1234",
		"abc":           "abc",
		"":              "",
		"abc1234":       "abc1234",
	}
	for full, want := range cases {
		got := PRInfo{HeadSHA: full}.ShortSHA()
		if got != want {
			t.Errorf("ShortSHA(%q) = %q, want %q", full, got, want)
		}
	}
}
