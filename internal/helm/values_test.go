package helm

import (
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
	for _, key := range []string{"replicas", "containerPort", "env", "tls"} {
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
}

func TestBuildValues_PRNumberIsAlwaysString(t *testing.T) {
	// The chart label deploy-pr/pr is a string; values must encode the number
	// as a string so the helper template doesn't have to coerce it.
	v := BuildValues(ValuesInput{PRNumber: 7, ImageRepository: "x", ImageTag: "y", Host: "h"})
	if got, ok := v["prNumber"].(string); !ok || got != "7" {
		t.Errorf("prNumber = %v (type %T), want \"7\" string", v["prNumber"], v["prNumber"])
	}
}
