package helm

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// ValuesInput is the typed shape of everything deploy-pr needs to fill into
// charts/preview's values. Constructing the map by hand from typed fields
// (instead of string-formatting YAML) keeps the schema validator honest.
type ValuesInput struct {
	PRNumber        int
	ShortSHA        string
	ImageRepository string
	ImageTag        string
	Host            string
	Replicas        int
	ContainerPort   int
	Env             map[string]string
	TLSEnabled      bool
	PostgresEnabled bool
}

// BuildValues converts a ValuesInput to the nested map[string]any shape
// Helm's templating engine consumes. Zero-valued fields are omitted so the
// chart's defaults apply.
func BuildValues(in ValuesInput) map[string]any {
	v := map[string]any{
		"image": map[string]any{
			"repository": in.ImageRepository,
			"tag":        in.ImageTag,
		},
		"host":     in.Host,
		"prNumber": strconv.Itoa(in.PRNumber),
		"sha":      in.ShortSHA,
	}
	if in.Replicas > 0 {
		v["replicas"] = in.Replicas
	}
	if in.ContainerPort > 0 {
		v["containerPort"] = in.ContainerPort
	}
	if len(in.Env) > 0 {
		envMap := make(map[string]any, len(in.Env))
		for k, val := range in.Env {
			envMap[k] = val
		}
		v["env"] = envMap
	}
	if in.TLSEnabled {
		v["tls"] = map[string]any{"enabled": true}
	}
	if in.PostgresEnabled {
		v["postgres"] = map[string]any{"enabled": true}
	}
	return v
}

// LoadValuesFile reads a Helm-style YAML values file into a map.
func LoadValuesFile(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read values file %q: %w", path, err)
	}
	var v map[string]any
	if err := yaml.Unmarshal(b, &v); err != nil {
		return nil, fmt.Errorf("parse values file %q: %w", path, err)
	}
	if v == nil {
		v = map[string]any{}
	}
	return v, nil
}

// MergeValues recursively overlays src onto dst and returns dst. It follows
// Helm values-file semantics for maps: nested maps are merged, scalars and
// lists are replaced.
func MergeValues(dst, src map[string]any) map[string]any {
	if dst == nil {
		dst = map[string]any{}
	}
	for k, srcVal := range src {
		srcMap, srcOK := srcVal.(map[string]any)
		dstMap, dstOK := dst[k].(map[string]any)
		if srcOK && dstOK {
			dst[k] = MergeValues(dstMap, srcMap)
			continue
		}
		dst[k] = srcVal
	}
	return dst
}
