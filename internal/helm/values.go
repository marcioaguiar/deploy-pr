package helm

import "strconv"

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
	return v
}
