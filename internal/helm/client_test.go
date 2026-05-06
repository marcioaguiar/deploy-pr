package helm

import (
	"context"
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
