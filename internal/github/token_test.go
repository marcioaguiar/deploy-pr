package github

import (
	"errors"
	"strings"
	"testing"
)

func TestTokenResolver_FlagWins(t *testing.T) {
	r := TokenResolver{
		Explicit: "flagtoken",
		Env:      func(string) string { return "envtoken" },
		GHToken:  func() (string, error) { return "ghtoken", nil },
	}
	tok, src, err := r.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if tok != "flagtoken" || src != SourceFlag {
		t.Errorf("got tok=%q src=%v, want flagtoken/flag", tok, src)
	}
}

func TestTokenResolver_EnvWhenNoFlag(t *testing.T) {
	r := TokenResolver{
		Env:     func(string) string { return "envtoken" },
		GHToken: func() (string, error) { return "ghtoken", nil },
	}
	tok, src, err := r.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if tok != "envtoken" || src != SourceEnv {
		t.Errorf("got tok=%q src=%v", tok, src)
	}
}

func TestTokenResolver_GHFallback(t *testing.T) {
	r := TokenResolver{
		Env:     func(string) string { return "" },
		GHToken: func() (string, error) { return "ghtoken", nil },
	}
	tok, src, err := r.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if tok != "ghtoken" || src != SourceGH {
		t.Errorf("got tok=%q src=%v", tok, src)
	}
}

func TestTokenResolver_NoSourcesIsError(t *testing.T) {
	r := TokenResolver{
		Env:     func(string) string { return "" },
		GHToken: func() (string, error) { return "", errors.New("gh: not authed") },
	}
	_, _, err := r.Resolve()
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrNoToken) {
		t.Errorf("expected ErrNoToken, got %v", err)
	}
	for _, want := range []string{"--github-token", "GITHUB_TOKEN", "gh"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q: %v", want, err)
		}
	}
}

func TestTokenResolver_TrimsEnvWhitespace(t *testing.T) {
	r := TokenResolver{
		Env: func(string) string { return "  padded  " },
	}
	tok, src, err := r.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if tok != "padded" || src != SourceEnv {
		t.Errorf("got tok=%q src=%v", tok, src)
	}
}
