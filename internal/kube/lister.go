// Package kube enumerates the cluster-side state deploy-pr's `list` command
// reports on. It reuses the Helm SDK for release info (which is the source of
// truth for an active preview) and falls back to the kube namespace listing
// to surface orphans -- namespaces that match the prefix but have no
// associated Helm release.
package kube

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/marcioaguiar/deploy-pr/internal/helm"
)

// Preview describes one row of `deploy-pr list` output.
type Preview struct {
	PRNumber  int       `json:"pr"`
	Namespace string    `json:"namespace"`
	Release   string    `json:"release,omitempty"`
	ImageTag  string    `json:"image_tag,omitempty"`
	URL       string    `json:"url,omitempty"`
	Age       string    `json:"age,omitempty"`
	Updated   time.Time `json:"updated,omitempty"`
	Status    string    `json:"status"`
}

// ListInput parameterises List so cmd/list.go does not need to know how the
// data is stitched together.
type ListInput struct {
	NamespacePrefix string
	BaseDomain      string
	Logger          *slog.Logger
}

// List returns previews sorted by PR number, plus orphaned namespaces (those
// that match the prefix but have no Helm release) at the end of the slice.
func List(ctx context.Context, in ListInput) ([]Preview, error) {
	if in.NamespacePrefix == "" {
		return nil, fmt.Errorf("NamespacePrefix is required")
	}

	releases, err := helm.ListReleasesByNamespacePrefix(in.NamespacePrefix, in.Logger)
	if err != nil {
		return nil, err
	}

	releasedNS := map[string]bool{}
	previews := make([]Preview, 0, len(releases))
	for _, r := range releases {
		releasedNS[r.Namespace] = true
		updated := r.Info.LastDeployed.Time
		previews = append(previews, Preview{
			PRNumber:  PRNumberFromNamespace(r.Namespace, in.NamespacePrefix),
			Namespace: r.Namespace,
			Release:   r.Name,
			ImageTag:  ImageTagFromValues(r.Config),
			URL:       buildURL(r.Namespace, in.BaseDomain),
			Age:       humanAge(updated),
			Updated:   updated,
			Status:    r.Info.Status.String(),
		})
	}

	namespaces, err := helm.ListNamespacesWithPrefix(ctx, in.NamespacePrefix)
	if err != nil {
		return nil, err
	}
	for _, ns := range namespaces {
		if releasedNS[ns] {
			continue
		}
		previews = append(previews, Preview{
			PRNumber:  PRNumberFromNamespace(ns, in.NamespacePrefix),
			Namespace: ns,
			URL:       buildURL(ns, in.BaseDomain),
			Status:    "orphaned",
		})
	}

	sort.SliceStable(previews, func(i, j int) bool {
		// Orphans last; otherwise by PR number ascending.
		oi, oj := previews[i].Status == "orphaned", previews[j].Status == "orphaned"
		if oi != oj {
			return !oi
		}
		return previews[i].PRNumber < previews[j].PRNumber
	})

	return previews, nil
}

// PRNumberFromNamespace pulls the integer suffix off a namespace named
// "<prefix><N>". Returns 0 when the suffix isn't a number.
func PRNumberFromNamespace(ns, prefix string) int {
	rest := strings.TrimPrefix(ns, prefix)
	n, err := strconv.Atoi(rest)
	if err != nil {
		return 0
	}
	return n
}

// ImageTagFromValues digs image.tag out of a Helm values map. Returns ""
// when not present (rare; only orphaned-by-config releases).
func ImageTagFromValues(values map[string]any) string {
	img, ok := values["image"].(map[string]any)
	if !ok {
		return ""
	}
	tag, _ := img["tag"].(string)
	return tag
}

func buildURL(ns, baseDomain string) string {
	if baseDomain == "" {
		return ""
	}
	return fmt.Sprintf("https://%s.%s", ns, baseDomain)
}

// humanAge turns a deployment time into a compact "3h", "2d" string.
func humanAge(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours())/24)
	}
}
