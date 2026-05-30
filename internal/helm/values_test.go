package helm

import (
	"os"
	"reflect"
	"testing"
)

func TestBuildValues_CanonicalShape(t *testing.T) {
	v := BuildValues(ValuesInput{
		PRNumber:        42,
		ShortSHA:        "abc1234",
		ImageRepository: "111111111111.dkr.ecr.us-east-1.amazonaws.com/myapp",
		ImageTag:        "pr-42-abc1234",
		Host:            "pr-42.preview.example.com",
	})

	want := map[string]any{
		"image": map[string]any{
			"repository": "111111111111.dkr.ecr.us-east-1.amazonaws.com/myapp",
			"tag":        "pr-42-abc1234",
		},
		"host":     "pr-42.preview.example.com",
		"prNumber": "42",
		"sha":      "abc1234",
	}
	if !reflect.DeepEqual(v, want) {
		t.Errorf("values mismatch:\n got %#v\nwant %#v", v, want)
	}
}

func TestBuildValues_OptionalFieldsOmittedWhenZero(t *testing.T) {
	v := BuildValues(ValuesInput{
		PRNumber:        1,
		ImageRepository: "x",
		ImageTag:        "y",
		Host:            "h",
	})
	for _, key := range []string{"replicas", "containerPort", "env", "tls", "postgres"} {
		if _, ok := v[key]; ok {
			t.Errorf("%q should be omitted when not set", key)
		}
	}
}

func TestBuildValues_OptionalFieldsRoundTrip(t *testing.T) {
	v := BuildValues(ValuesInput{
		PRNumber:        1,
		ShortSHA:        "s",
		ImageRepository: "x",
		ImageTag:        "y",
		Host:            "h",
		Replicas:        3,
		ContainerPort:   3000,
		Env:             map[string]string{"FOO": "bar"},
		TLSEnabled:      true,
		PostgresEnabled: true,
	})
	if v["replicas"] != 3 {
		t.Errorf("replicas = %v", v["replicas"])
	}
	if v["containerPort"] != 3000 {
		t.Errorf("containerPort = %v", v["containerPort"])
	}
	envMap, ok := v["env"].(map[string]any)
	if !ok || envMap["FOO"] != "bar" {
		t.Errorf("env = %v", v["env"])
	}
	tlsMap, ok := v["tls"].(map[string]any)
	if !ok || tlsMap["enabled"] != true {
		t.Errorf("tls = %v", v["tls"])
	}
	postgresMap, ok := v["postgres"].(map[string]any)
	if !ok || postgresMap["enabled"] != true {
		t.Errorf("postgres = %v", v["postgres"])
	}
}

func TestBuildValues_PRNumberIsAlwaysString(t *testing.T) {
	// The chart label deploy-pr/pr is a string; values must encode the number
	// as a string so the helper template doesn't have to coerce it.
	v := BuildValues(ValuesInput{PRNumber: 7, ImageRepository: "x", ImageTag: "y", Host: "h"})
	if got, ok := v["prNumber"].(string); !ok || got != "7" {
		t.Errorf("prNumber = %v (type %T), want \"7\" string", v["prNumber"], v["prNumber"])
	}
}

func TestMergeValues_RecursivelyOverlaysMaps(t *testing.T) {
	got := MergeValues(
		map[string]any{
			"image": map[string]any{
				"repository": "repo",
				"tag":        "old",
			},
			"envFrom": []any{"old"},
		},
		map[string]any{
			"image": map[string]any{
				"tag": "new",
			},
			"envFrom": []any{"new"},
		},
	)

	image := got["image"].(map[string]any)
	if image["repository"] != "repo" || image["tag"] != "new" {
		t.Errorf("image merge = %#v", image)
	}
	if !reflect.DeepEqual(got["envFrom"], []any{"new"}) {
		t.Errorf("envFrom = %#v", got["envFrom"])
	}
}

func TestLoadValuesFile(t *testing.T) {
	path := t.TempDir() + "/values.yaml"
	if err := os.WriteFile(path, []byte("postgres:\n  enabled: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadValuesFile(path)
	if err != nil {
		t.Fatalf("load values: %v", err)
	}
	postgres := got["postgres"].(map[string]any)
	if postgres["enabled"] != true {
		t.Errorf("postgres.enabled = %v", postgres["enabled"])
	}
}
