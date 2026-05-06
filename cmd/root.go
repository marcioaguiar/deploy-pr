// Package cmd builds the deploy-pr Cobra command tree and owns global
// configuration loading and logger construction. Subcommand handlers in this
// package compose the work in internal/* packages.
package cmd

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/marcioaguiar/deploy-pr/internal/config"
)

// Execute is the process entry point. It runs the root command and exits
// with a code derived from the returned error.
func Execute() {
	root := NewRootCmd(os.Stdout, os.Stderr)
	err := root.Execute()
	os.Exit(exitCodeFor(err))
}

// rootOpts holds shared state populated in PersistentPreRunE so that
// subcommand RunE functions can read configuration and log without
// re-loading.
type rootOpts struct {
	configPath  string
	logFormat   string
	verbose     bool
	repo        string
	githubToken string

	cfg    *config.Config
	logger *slog.Logger
	stdout io.Writer
	stderr io.Writer
}

// NewRootCmd builds a fresh root command tree wired to the provided writers.
// Tests construct a tree per invocation so they can capture output.
func NewRootCmd(stdout, stderr io.Writer) *cobra.Command {
	opts := &rootOpts{
		stdout: stdout,
		stderr: stderr,
	}

	cmd := &cobra.Command{
		Use:   "deploy-pr",
		Short: "Deploy a GitHub PR to an ephemeral Kubernetes preview environment on EKS",
		Long: "deploy-pr turns a GitHub pull request into a self-contained ephemeral preview\n" +
			"environment on Amazon EKS: it resolves the PR's head SHA, builds and pushes a\n" +
			"container image to ECR, then installs a Helm release into a per-PR namespace.\n" +
			"The same binary works on a developer laptop and inside GitHub Actions.",
		SilenceUsage:  true,
		SilenceErrors: false,
		PersistentPreRunE: func(c *cobra.Command, _ []string) error {
			cfg, err := config.Load(opts.configPath)
			if err != nil {
				return userErr(fmt.Errorf("load config: %w", err))
			}
			if opts.repo != "" {
				cfg.Repo = opts.repo
			}
			if opts.logFormat != "" {
				cfg.LogFormat = opts.logFormat
			}
			opts.cfg = cfg
			opts.logger = newLogger(opts.stderr, cfg.LogFormat, opts.verbose)
			return nil
		},
	}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	cmd.PersistentFlags().StringVar(&opts.configPath, "config", "", "path to config file (default .deploy-pr.yaml in current dir)")
	cmd.PersistentFlags().StringVar(&opts.logFormat, "log-format", "", "log format: text or json (overrides config)")
	cmd.PersistentFlags().BoolVarP(&opts.verbose, "verbose", "v", false, "enable verbose (debug) logs")
	cmd.PersistentFlags().StringVar(&opts.repo, "repo", "", "GitHub repo as owner/name (overrides git remote auto-detect)")
	cmd.PersistentFlags().StringVar(&opts.githubToken, "github-token", "", "GitHub token (default: GITHUB_TOKEN env, then `gh auth token`)")

	cmd.AddCommand(newUpCmd(opts))
	cmd.AddCommand(newDownCmd(opts))
	cmd.AddCommand(newListCmd(opts))
	cmd.AddCommand(newVersionCmd(stdout))

	return cmd
}

func newLogger(w io.Writer, format string, verbose bool) *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	hopts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if format == "json" {
		h = slog.NewJSONHandler(w, hopts)
	} else {
		h = slog.NewTextHandler(w, hopts)
	}
	return slog.New(h)
}
