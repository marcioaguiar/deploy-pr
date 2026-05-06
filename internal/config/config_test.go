package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// allEnvVars is the set of env vars the package may read. Tests clear all of
// them so a developer's shell environment does not leak into assertions.
var allEnvVars = []string{
	"DEPLOY_PR_REPO",
	"DEPLOY_PR_ECR_REPO_URI",
	"DEPLOY_PR_REGION",
	"DEPLOY_PR_CLUSTER_NAME",
	"DEPLOY_PR_BASE_DOMAIN",
	"DEPLOY_PR_CHART_PATH",
	"DEPLOY_PR_NAMESPACE_PREFIX",
	"DEPLOY_PR_TIMEOUT",
	"DEPLOY_PR_LOG_FORMAT",
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range allEnvVars {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
}

// chdir switches to dir for the duration of the test.
func chdir(t *testing.T, dir string) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
}

func TestLoad_DefaultsWhenNoSources(t *testing.T) {
	clearEnv(t)
	chdir(t, t.TempDir())

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Namespace.Prefix != "pr-" {
		t.Errorf("Namespace.Prefix = %q, want %q", cfg.Namespace.Prefix, "pr-")
	}
	if cfg.ChartPath != "charts/preview" {
		t.Errorf("ChartPath = %q, want %q", cfg.ChartPath, "charts/preview")
	}
	if cfg.LogFormat != "text" {
		t.Errorf("LogFormat = %q, want text", cfg.LogFormat)
	}
	if cfg.Region != "us-east-1" {
		t.Errorf("Region = %q, want us-east-1", cfg.Region)
	}
}

func TestLoad_FileSuppliesValues(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	chdir(t, dir)

	cfgPath := filepath.Join(dir, ".deploy-pr.yaml")
	body := `
base_domain: file.example.com
ecr_repo_uri: 123456789012.dkr.ecr.us-east-1.amazonaws.com/myapp
cluster_name: dev-cluster
namespace:
  prefix: preview-
`
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BaseDomain != "file.example.com" {
		t.Errorf("BaseDomain = %q", cfg.BaseDomain)
	}
	if cfg.Namespace.Prefix != "preview-" {
		t.Errorf("Namespace.Prefix = %q", cfg.Namespace.Prefix)
	}
}

func TestLoad_EnvOverridesFile(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	chdir(t, dir)

	cfgPath := filepath.Join(dir, ".deploy-pr.yaml")
	if err := os.WriteFile(cfgPath, []byte("base_domain: file.example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEPLOY_PR_BASE_DOMAIN", "env.example.com")

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BaseDomain != "env.example.com" {
		t.Errorf("BaseDomain = %q, want env.example.com", cfg.BaseDomain)
	}
}

func TestLoad_NestedEnvOverride(t *testing.T) {
	clearEnv(t)
	chdir(t, t.TempDir())

	t.Setenv("DEPLOY_PR_NAMESPACE_PREFIX", "ns-")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Namespace.Prefix != "ns-" {
		t.Errorf("Namespace.Prefix = %q, want ns-", cfg.Namespace.Prefix)
	}
}

func TestLoad_MissingFileNotAnError(t *testing.T) {
	clearEnv(t)
	chdir(t, t.TempDir())

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg == nil {
		t.Fatal("cfg is nil")
	}
}

func TestValidateForUp_ReportsAllMissingFields(t *testing.T) {
	cfg := &Config{}
	err := cfg.ValidateForUp()
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"ecr_repo_uri", "cluster_name", "base_domain"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}
}

func TestValidateForUp_OKWhenAllSet(t *testing.T) {
	cfg := &Config{
		ECRRepoURI:  "x",
		ClusterName: "y",
		BaseDomain:  "z",
	}
	if err := cfg.ValidateForUp(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
