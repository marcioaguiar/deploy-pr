package helm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

// NewManager exercises kubeconfig discovery and may legitimately fail when
// no kube context is configured (CI). The point of the test is that the
// surface compiles and rejects bad inputs predictably -- not that it talks
// to a cluster.
func TestUpgradeOrInstall_RejectsBadInputs(t *testing.T) {
	m := &Manager{log: slog.Default()}

	if _, err := m.UpgradeOrInstall(context.Background(), UpgradeOptions{}); err == nil {
		t.Error("expected error for empty options")
	}
	if _, err := m.UpgradeOrInstall(context.Background(), UpgradeOptions{ReleaseName: "x"}); err == nil {
		t.Error("expected error when chart is nil")
	}
}

func TestNewManager_BadKubeconfigSurfaceIsClean(t *testing.T) {
	// Point KUBECONFIG at a path we know does not exist; NewManager must
	// either succeed (some loaders are lenient) or return a clear error,
	// not panic.
	t.Setenv("KUBECONFIG", "/nonexistent/path/to/kubeconfig")
	_, err := NewManager("pr-1", slog.Default())
	if err != nil && !strings.Contains(err.Error(), "config") {
		t.Errorf("unexpected error shape: %v", err)
	}
}

func TestAnnotateUnreachable(t *testing.T) {
	sentinel := errors.New("Kubernetes cluster unreachable: dial tcp 127.0.0.1:8080: connect: connection refused")

	tests := []struct {
		name       string
		in         error
		wantHint   bool
		wantSameAs error
	}{
		{name: "nil", in: nil, wantHint: false},
		{name: "helm wrapper", in: errors.New("history pr-3075: Kubernetes cluster unreachable: ..."), wantHint: true},
		{name: "raw localhost dial", in: errors.New("dial tcp 127.0.0.1:8080: connect: connection refused"), wantHint: true},
		{name: "localhost host", in: errors.New(`Get "http://localhost:8080/version": connection refused`), wantHint: true},
		{name: "unrelated error", in: errors.New("release: not found"), wantHint: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := annotateUnreachable(tc.in)
			if tc.in == nil {
				if got != nil {
					t.Fatalf("nil input must return nil, got %v", got)
				}
				return
			}
			has := strings.Contains(got.Error(), kubeUnreachableHint)
			if has != tc.wantHint {
				t.Fatalf("hint present = %v, want %v (err=%q)", has, tc.wantHint, got.Error())
			}
			if !tc.wantHint && got.Error() != tc.in.Error() {
				t.Fatalf("unrelated error must pass through unchanged: got %q want %q", got.Error(), tc.in.Error())
			}
		})
	}

	t.Run("idempotent", func(t *testing.T) {
		first := annotateUnreachable(sentinel)
		second := annotateUnreachable(first)
		if c := strings.Count(second.Error(), kubeUnreachableHint); c != 1 {
			t.Fatalf("hint should appear exactly once after double-wrap, got %d", c)
		}
	})

	t.Run("preserves error chain", func(t *testing.T) {
		wrapped := fmt.Errorf("history pr-1: %w", sentinel)
		got := annotateUnreachable(wrapped)
		if !errors.Is(got, sentinel) {
			t.Fatalf("annotated error must still match original via errors.Is")
		}
	})
}
