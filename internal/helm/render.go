// Package helm wraps Helm's Go SDK with the small surface deploy-pr needs:
// load a chart from disk, render it with values for diagnostics, and (in U5)
// install/upgrade/uninstall via action.NewInstall etc.
package helm

import (
	"fmt"
	"sort"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/engine"
)

// LoadChart reads a chart from disk. The path may be a directory (for our
// in-tree charts/preview) or a packaged .tgz.
func LoadChart(path string) (*chart.Chart, error) {
	c, err := loader.Load(path)
	if err != nil {
		return nil, fmt.Errorf("load chart %q: %w", path, err)
	}
	return c, nil
}

// RenderOptions controls the in-process rendering deploy-pr does for tests
// and for `--dry-run` previews. It mirrors the surface of action.Install.Run
// minus everything that requires a live cluster.
type RenderOptions struct {
	ReleaseName string
	Namespace   string
	Values      map[string]any
}

// Render executes the chart's templates against the given options and
// returns a map keyed by template path (e.g. "preview/templates/ingress.yaml")
// with the rendered YAML as the value. It does not contact a cluster.
func Render(c *chart.Chart, opts RenderOptions) (map[string]string, error) {
	if opts.ReleaseName == "" {
		return nil, fmt.Errorf("ReleaseName is required")
	}
	if opts.Namespace == "" {
		return nil, fmt.Errorf("Namespace is required")
	}

	// Validate values against the chart's schema before rendering. This is
	// the same gate `helm install` runs.
	if err := chartutil.ValidateAgainstSchema(c, opts.Values); err != nil {
		return nil, fmt.Errorf("values schema validation: %w", err)
	}

	finalValues, err := chartutil.ToRenderValues(c,
		opts.Values,
		chartutil.ReleaseOptions{
			Name:      opts.ReleaseName,
			Namespace: opts.Namespace,
			IsInstall: true,
		},
		nil)
	if err != nil {
		return nil, fmt.Errorf("compose render values: %w", err)
	}

	rendered, err := engine.Render(c, finalValues)
	if err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}

	// Drop empty files (e.g. namespace.yaml when createNamespace=false) and
	// any non-template files Helm includes by default.
	out := map[string]string{}
	for k, v := range rendered {
		if isBlank(v) {
			continue
		}
		out[k] = v
	}
	return out, nil
}

// SortedKeys returns the rendered template paths in deterministic order so
// callers can iterate without map randomness leaking into output.
func SortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func isBlank(s string) bool {
	for _, r := range s {
		switch r {
		case ' ', '\t', '\n', '\r':
			continue
		default:
			return false
		}
	}
	return true
}
