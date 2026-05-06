package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRenderBody_Ready(t *testing.T) {
	body := CommentInput{
		Status:    StatusReady,
		URL:       "https://pr-1.preview.example.com",
		ImageTag:  "myapp:pr-1-abc1234",
		Timestamp: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
	}.RenderBody()

	if !strings.HasPrefix(body, stickyMarker) {
		t.Errorf("body must start with marker:\n%s", body)
	}
	for _, want := range []string{"Preview ready", "pr-1.preview.example.com", "myapp:pr-1-abc1234", "2026-05-01T12:00:00Z"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
}

func TestRenderBody_TornDown(t *testing.T) {
	body := CommentInput{
		Status:    StatusTornDown,
		URL:       "https://pr-1.preview.example.com",
		Timestamp: time.Date(2026, 5, 2, 9, 0, 0, 0, time.UTC),
	}.RenderBody()

	if !strings.HasPrefix(body, stickyMarker) {
		t.Errorf("body must start with marker")
	}
	if !strings.Contains(body, "Preview torn down") {
		t.Errorf("missing torn-down text:\n%s", body)
	}
	if !strings.Contains(body, "~~") {
		t.Errorf("missing strikethrough:\n%s", body)
	}
	if !strings.Contains(body, "2026-05-02T09:00:00Z") {
		t.Errorf("missing timestamp:\n%s", body)
	}
}

func TestUpsertStickyComment_CreatesWhenNoneExists(t *testing.T) {
	var createdBody string
	var createCount, listCount int32

	c, _ := newTestClient(t, SourceEnv, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/v3/repos/octo/widgets/issues/42/comments", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				atomic.AddInt32(&listCount, 1)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintln(w, `[]`)
			case http.MethodPost:
				atomic.AddInt32(&createCount, 1)
				var c struct {
					Body string `json:"body"`
				}
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &c)
				createdBody = c.Body
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				fmt.Fprintln(w, `{"id": 999}`)
			default:
				t.Errorf("unexpected method: %s", r.Method)
			}
		})
	})

	id, err := c.UpsertStickyComment(context.Background(), "octo", "widgets", 42, "BODY")
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if id != 999 {
		t.Errorf("id = %d", id)
	}
	if listCount != 1 || createCount != 1 {
		t.Errorf("call counts: list=%d create=%d", listCount, createCount)
	}
	if createdBody != "BODY" {
		t.Errorf("body = %q", createdBody)
	}
}

func TestUpsertStickyComment_UpdatesWhenMarkerFound(t *testing.T) {
	var editedBody string
	var editID int64

	c, _ := newTestClient(t, SourceEnv, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/v3/repos/octo/widgets/issues/42/comments", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("unexpected method on list: %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `[
				{"id": 100, "body": "unrelated"},
				{"id": 200, "body": "<!-- deploy-pr:preview -->\n\nold body"},
				{"id": 300, "body": "another"}
			]`)
		})
		mux.HandleFunc("/api/v3/repos/octo/widgets/issues/comments/200", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPatch {
				t.Errorf("expected PATCH, got %s", r.Method)
			}
			var c struct {
				Body string `json:"body"`
			}
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &c)
			editedBody = c.Body
			editID = 200
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"id": 200}`)
		})
	})

	id, err := c.UpsertStickyComment(context.Background(), "octo", "widgets", 42, "NEW BODY")
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if id != 200 {
		t.Errorf("id = %d, want 200", id)
	}
	if editID != 200 || editedBody != "NEW BODY" {
		t.Errorf("edit not as expected: id=%d body=%q", editID, editedBody)
	}
}

func TestUpsertStickyComment_PicksFirstMarkerWhenMultiple(t *testing.T) {
	var seenID int64

	c, _ := newTestClient(t, SourceEnv, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/v3/repos/octo/widgets/issues/42/comments", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `[
				{"id": 50, "body": "<!-- deploy-pr:preview -->\nfirst"},
				{"id": 60, "body": "<!-- deploy-pr:preview -->\nsecond"}
			]`)
		})
		mux.HandleFunc("/api/v3/repos/octo/widgets/issues/comments/", func(w http.ResponseWriter, r *http.Request) {
			seenID = parseTrailingID(r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `{"id": 50}`)
		})
	})

	_, err := c.UpsertStickyComment(context.Background(), "octo", "widgets", 42, "X")
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if seenID != 50 {
		t.Errorf("expected to update first match (id=50), updated id=%d", seenID)
	}
}

func TestUpsertStickyComment_403WrappedAsAuthFailed(t *testing.T) {
	c, _ := newTestClient(t, SourceEnv, func(mux *http.ServeMux) {
		mux.HandleFunc("/api/v3/repos/octo/widgets/issues/42/comments", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, `{"message":"Resource not accessible"}`, http.StatusForbidden)
		})
	})

	_, err := c.UpsertStickyComment(context.Background(), "octo", "widgets", 42, "X")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrAuthFailed) {
		t.Errorf("expected ErrAuthFailed, got %v", err)
	}
}

// parseTrailingID extracts the trailing integer from a path like
// /api/v3/repos/octo/widgets/issues/comments/50.
func parseTrailingID(path string) int64 {
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	last := parts[len(parts)-1]
	var n int64
	for _, r := range last {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int64(r-'0')
	}
	return n
}
