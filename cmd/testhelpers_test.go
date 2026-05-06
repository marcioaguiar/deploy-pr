package cmd

import (
	"os"
	"testing"
)

// chdirTemp moves the test into a fresh temp directory so config file lookups
// (which default to ".deploy-pr.yaml" in cwd) don't pick up the developer's
// real config.
func chdirTemp(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
}

func clearTestEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"DEPLOY_PR_REPO",
		"DEPLOY_PR_ECR_REPO_URI",
		"DEPLOY_PR_REGION",
		"DEPLOY_PR_CLUSTER_NAME",
		"DEPLOY_PR_BASE_DOMAIN",
		"DEPLOY_PR_CHART_PATH",
		"DEPLOY_PR_NAMESPACE_PREFIX",
		"DEPLOY_PR_TIMEOUT",
		"DEPLOY_PR_LOG_FORMAT",
	} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
}
