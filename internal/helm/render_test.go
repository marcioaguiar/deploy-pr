package helm

import (
	"path/filepath"
	"strings"
	"testing"
)

const chartRelPath = "../../charts/preview"

func loadPreview(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(chartRelPath)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func TestPreviewChart_Loads(t *testing.T) {
	c, err := LoadChart(loadPreview(t))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.Metadata.Name != "preview" {
		t.Errorf("name = %q", c.Metadata.Name)
	}
}

func TestPreviewChart_DefaultValuesProduceCoreResources(t *testing.T) {
	c, err := LoadChart(loadPreview(t))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	out, err := Render(c, RenderOptions{
		ReleaseName: "pr-1",
		Namespace:   "pr-1",
		Values: map[string]any{
			"image": map[string]any{
				"repository": "111111111111.dkr.ecr.us-east-1.amazonaws.com/myapp",
				"tag":        "pr-1-abc1234",
			},
			"host":     "pr-1.preview.example.com",
			"prNumber": "1",
			"sha":      "abc1234",
		},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	want := []string{
		"preview/templates/deployment.yaml",
		"preview/templates/service.yaml",
		"preview/templates/ingress.yaml",
	}
	for _, p := range want {
		if _, ok := out[p]; !ok {
			t.Errorf("missing template %q in render output (have: %v)", p, SortedKeys(out))
		}
	}
	// Default createNamespace=false -> namespace.yaml should be absent (blank).
	if _, ok := out["preview/templates/namespace.yaml"]; ok {
		t.Errorf("namespace.yaml unexpectedly rendered with createNamespace=false")
	}

	// Spot-check substantive content.
	deploy := out["preview/templates/deployment.yaml"]
	if !strings.Contains(deploy, "image: \"111111111111.dkr.ecr.us-east-1.amazonaws.com/myapp:pr-1-abc1234\"") {
		t.Errorf("deployment image not interpolated:\n%s", deploy)
	}
	if !strings.Contains(deploy, "app.kubernetes.io/managed-by: deploy-pr") {
		t.Errorf("managed-by label missing:\n%s", deploy)
	}
	if !strings.Contains(deploy, "deploy-pr/pr: \"1\"") {
		t.Errorf("deploy-pr/pr label missing:\n%s", deploy)
	}

	ingress := out["preview/templates/ingress.yaml"]
	if !strings.Contains(ingress, "external-dns.alpha.kubernetes.io/hostname: \"pr-1.preview.example.com\"") {
		t.Errorf("ingress hostname annotation missing:\n%s", ingress)
	}
	if !strings.Contains(ingress, "host: \"pr-1.preview.example.com\"") {
		t.Errorf("ingress rule host missing:\n%s", ingress)
	}
	if strings.Contains(ingress, "tls:") {
		t.Errorf("tls block rendered when tls.enabled=false:\n%s", ingress)
	}
}

func TestPreviewChart_TLSEnabledRendersTLSBlock(t *testing.T) {
	c, _ := LoadChart(loadPreview(t))
	out, err := Render(c, RenderOptions{
		ReleaseName: "pr-2",
		Namespace:   "pr-2",
		Values: map[string]any{
			"image": map[string]any{"repository": "x/y", "tag": "v1"},
			"host":  "pr-2.preview.example.com",
			"tls":   map[string]any{"enabled": true},
		},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	ingress := out["preview/templates/ingress.yaml"]
	if !strings.Contains(ingress, "tls:") {
		t.Errorf("expected tls block:\n%s", ingress)
	}
	if !strings.Contains(ingress, "secretName: pr-2-preview-example-com-tls") {
		t.Errorf("expected derived secret name:\n%s", ingress)
	}
}

func TestPreviewChart_ReplicasAndEnvRender(t *testing.T) {
	c, _ := LoadChart(loadPreview(t))
	out, err := Render(c, RenderOptions{
		ReleaseName: "pr-3",
		Namespace:   "pr-3",
		Values: map[string]any{
			"image":    map[string]any{"repository": "x/y", "tag": "v1"},
			"host":     "pr-3.preview.example.com",
			"replicas": 3,
			"env": map[string]any{
				"FOO": "bar",
				"BAZ": "qux",
			},
		},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	deploy := out["preview/templates/deployment.yaml"]
	if !strings.Contains(deploy, "replicas: 3") {
		t.Errorf("replicas not rendered:\n%s", deploy)
	}
	if !strings.Contains(deploy, "name: FOO") || !strings.Contains(deploy, "value: \"bar\"") {
		t.Errorf("env FOO=bar missing:\n%s", deploy)
	}
	if !strings.Contains(deploy, "name: BAZ") || !strings.Contains(deploy, "value: \"qux\"") {
		t.Errorf("env BAZ=qux missing:\n%s", deploy)
	}
}

func TestPreviewChart_EnvFromRenders(t *testing.T) {
	c, _ := LoadChart(loadPreview(t))
	out, err := Render(c, RenderOptions{
		ReleaseName: "pr-31",
		Namespace:   "pr-31",
		Values: map[string]any{
			"image": map[string]any{"repository": "x/y", "tag": "v1"},
			"host":  "pr-31.preview.example.com",
			"envFrom": []any{
				map[string]any{
					"secretRef": map[string]any{"name": "preview-database-url"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	deploy := out["preview/templates/deployment.yaml"]
	if !strings.Contains(deploy, "envFrom:") || !strings.Contains(deploy, "name: preview-database-url") {
		t.Errorf("envFrom secret missing:\n%s", deploy)
	}
}

func TestPreviewChart_PostgresEnabledRendersDatabaseResources(t *testing.T) {
	c, _ := LoadChart(loadPreview(t))
	out, err := Render(c, RenderOptions{
		ReleaseName: "pr-32",
		Namespace:   "pr-32",
		Values: map[string]any{
			"image":    map[string]any{"repository": "x/y", "tag": "v1"},
			"host":     "pr-32.preview.example.com",
			"postgres": map[string]any{"enabled": true},
		},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, p := range []string{
		"preview/templates/postgres-secret.yaml",
		"preview/templates/postgres-service.yaml",
		"preview/templates/postgres-statefulset.yaml",
	} {
		if _, ok := out[p]; !ok {
			t.Errorf("missing postgres template %q in render output (have: %v)", p, SortedKeys(out))
		}
	}
	deploy := out["preview/templates/deployment.yaml"]
	if !strings.Contains(deploy, "name: DATABASE_URL") || !strings.Contains(deploy, "name: pr-32-postgres") {
		t.Errorf("deployment DATABASE_URL secret ref missing:\n%s", deploy)
	}
	secret := out["preview/templates/postgres-secret.yaml"]
	if !strings.Contains(secret, "DATABASE_URL: \"postgres://app:") {
		t.Errorf("secret DATABASE_URL missing:\n%s", secret)
	}
	if !strings.Contains(out["preview/templates/postgres-statefulset.yaml"], "image: \"postgres:16\"") {
		t.Errorf("postgres image missing:\n%s", out["preview/templates/postgres-statefulset.yaml"])
	}
}

func TestPreviewChart_CreateNamespaceTrue(t *testing.T) {
	c, _ := LoadChart(loadPreview(t))
	out, err := Render(c, RenderOptions{
		ReleaseName: "pr-4",
		Namespace:   "pr-4",
		Values: map[string]any{
			"image":           map[string]any{"repository": "x/y", "tag": "v1"},
			"host":            "pr-4.preview.example.com",
			"createNamespace": true,
		},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	ns, ok := out["preview/templates/namespace.yaml"]
	if !ok {
		t.Fatalf("expected namespace.yaml to render with createNamespace=true. keys=%v", SortedKeys(out))
	}
	if !strings.Contains(ns, "name: pr-4") || !strings.Contains(ns, "kind: Namespace") {
		t.Errorf("namespace template content unexpected:\n%s", ns)
	}
}

func TestPreviewChart_SchemaRejectsEmptyImageTag(t *testing.T) {
	c, _ := LoadChart(loadPreview(t))
	_, err := Render(c, RenderOptions{
		ReleaseName: "pr-5",
		Namespace:   "pr-5",
		Values: map[string]any{
			"image": map[string]any{"repository": "x/y", "tag": ""},
			"host":  "pr-5.preview.example.com",
		},
	})
	if err == nil {
		t.Fatal("expected schema validation error for empty image.tag")
	}
	if !strings.Contains(err.Error(), "schema") && !strings.Contains(err.Error(), "tag") {
		t.Errorf("error should mention schema or tag: %v", err)
	}
}

func TestPreviewChart_SchemaRejectsMissingHost(t *testing.T) {
	c, _ := LoadChart(loadPreview(t))
	_, err := Render(c, RenderOptions{
		ReleaseName: "pr-6",
		Namespace:   "pr-6",
		Values: map[string]any{
			"image": map[string]any{"repository": "x/y", "tag": "v1"},
		},
	})
	if err == nil {
		t.Fatal("expected error for missing required host")
	}
}

func TestRender_RequiresReleaseAndNamespace(t *testing.T) {
	c, _ := LoadChart(loadPreview(t))
	if _, err := Render(c, RenderOptions{Namespace: "x"}); err == nil {
		t.Error("expected error for empty ReleaseName")
	}
	if _, err := Render(c, RenderOptions{ReleaseName: "x"}); err == nil {
		t.Error("expected error for empty Namespace")
	}
}
