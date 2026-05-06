package kube

import "testing"

func TestPRNumberFromNamespace(t *testing.T) {
	cases := []struct {
		ns, prefix string
		want       int
	}{
		{"pr-1", "pr-", 1},
		{"pr-123", "pr-", 123},
		{"preview-7", "preview-", 7},
		{"pr-abc", "pr-", 0},
		{"unrelated", "pr-", 0},
	}
	for _, tc := range cases {
		if got := PRNumberFromNamespace(tc.ns, tc.prefix); got != tc.want {
			t.Errorf("PRNumberFromNamespace(%q, %q) = %d, want %d", tc.ns, tc.prefix, got, tc.want)
		}
	}
}

func TestImageTagFromValues(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]any
		want string
	}{
		{"present", map[string]any{"image": map[string]any{"tag": "pr-1-abc1234"}}, "pr-1-abc1234"},
		{"missing tag", map[string]any{"image": map[string]any{"repository": "x"}}, ""},
		{"missing image", map[string]any{}, ""},
		{"wrong type", map[string]any{"image": "not a map"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ImageTagFromValues(tc.in); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildURL(t *testing.T) {
	if got := buildURL("pr-1", "preview.example.com"); got != "https://pr-1.preview.example.com" {
		t.Errorf("got %q", got)
	}
	if got := buildURL("pr-1", ""); got != "" {
		t.Errorf("expected empty URL when base domain is empty, got %q", got)
	}
}

func TestList_RequiresNamespacePrefix(t *testing.T) {
	_, err := List(nil, ListInput{NamespacePrefix: ""})
	if err == nil {
		t.Error("expected error for empty NamespacePrefix")
	}
}
