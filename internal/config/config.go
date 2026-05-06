// Package config loads deploy-pr's runtime configuration from a YAML file,
// environment variables (DEPLOY_PR_*), and explicit overrides, in that
// order of increasing precedence.
package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all runtime knobs for the CLI.
type Config struct {
	Repo        string        `mapstructure:"repo"`
	ECRRepoURI  string        `mapstructure:"ecr_repo_uri"`
	Region      string        `mapstructure:"region"`
	ClusterName string        `mapstructure:"cluster_name"`
	BaseDomain  string        `mapstructure:"base_domain"`
	ChartPath   string        `mapstructure:"chart_path"`
	Namespace   NamespaceCfg  `mapstructure:"namespace"`
	Timeout     time.Duration `mapstructure:"timeout"`
	LogFormat   string        `mapstructure:"log_format"`
}

type NamespaceCfg struct {
	Prefix string `mapstructure:"prefix"`
}

// envBindings maps each Config field's mapstructure key to the env var Viper
// should resolve it from. Nested keys (e.g. namespace.prefix) require explicit
// binding because Viper's AutomaticEnv does not reliably traverse nested
// structs even with a key replacer.
var envBindings = []string{
	"repo",
	"ecr_repo_uri",
	"region",
	"cluster_name",
	"base_domain",
	"chart_path",
	"namespace.prefix",
	"timeout",
	"log_format",
}

// Load reads configuration. If path is empty, Load looks for .deploy-pr.yaml in
// the current directory. A missing config file is not an error -- env vars and
// flag overrides may supply everything.
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetEnvPrefix("DEPLOY_PR")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	v.SetDefault("namespace.prefix", "pr-")
	v.SetDefault("chart_path", "charts/preview")
	v.SetDefault("timeout", "10m")
	v.SetDefault("log_format", "text")
	v.SetDefault("region", "us-east-1")

	for _, k := range envBindings {
		_ = v.BindEnv(k)
	}

	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.SetConfigName(".deploy-pr")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
	}

	if err := v.ReadInConfig(); err != nil {
		var nfErr viper.ConfigFileNotFoundError
		if !errors.As(err, &nfErr) {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &cfg, nil
}

// ValidateForUp returns a user-facing error naming every required field that
// is empty for an `up` invocation.
func (c *Config) ValidateForUp() error {
	missing := []string{}
	if c.ECRRepoURI == "" {
		missing = append(missing, "ecr_repo_uri")
	}
	if c.ClusterName == "" {
		missing = append(missing, "cluster_name")
	}
	if c.BaseDomain == "" {
		missing = append(missing, "base_domain")
	}
	if len(missing) > 0 {
		return fmt.Errorf(
			"missing required config: %s (set via .deploy-pr.yaml, env DEPLOY_PR_*, or flags)",
			strings.Join(missing, ", "),
		)
	}
	return nil
}
