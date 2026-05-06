// Package docker shells out to `docker buildx` to build and push a container
// image. Re-implementing BuildKit in-process is not worth it here; the CLI
// requires Docker on PATH (documented as a prerequisite) and this thin
// wrapper exists mainly to make argument construction testable.
package docker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
)

// Runner abstracts process invocation so tests can replace it with a recorder
// without spawning real subprocesses.
type Runner interface {
	LookPath(name string) error
	Run(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error
}

// BuildOptions describes a single buildx build (with optional push).
type BuildOptions struct {
	ContextDir string   // directory to build from (typically the repo root)
	Dockerfile string   // optional override; defaults to "<ContextDir>/Dockerfile"
	Platform   string   // optional; defaults to "linux/amd64"
	Tags       []string // fully-qualified image references including tag
	Push       bool
}

// Builder runs docker buildx with the given Runner and writers.
type Builder struct {
	Runner Runner
	Out    io.Writer
	Err    io.Writer
}

// New returns a Builder backed by the real docker CLI.
func New(out, errWriter io.Writer) *Builder {
	return &Builder{Runner: &execRunner{}, Out: out, Err: errWriter}
}

// Build assembles a `docker buildx build` invocation and runs it.
func (b *Builder) Build(ctx context.Context, opts BuildOptions) error {
	if len(opts.Tags) == 0 {
		return errors.New("at least one image tag is required")
	}
	if opts.ContextDir == "" {
		return errors.New("build context directory is required")
	}
	runner := b.Runner
	if runner == nil {
		runner = &execRunner{}
	}
	if err := runner.LookPath("docker"); err != nil {
		return fmt.Errorf("docker CLI not on PATH (install Docker or run from CI): %w", err)
	}

	platform := opts.Platform
	if platform == "" {
		platform = "linux/amd64"
	}

	args := []string{"buildx", "build", "--platform", platform}
	if opts.Dockerfile != "" {
		args = append(args, "--file", opts.Dockerfile)
	}
	for _, t := range opts.Tags {
		args = append(args, "--tag", t)
	}
	if opts.Push {
		args = append(args, "--push")
	}
	args = append(args, opts.ContextDir)

	out := writerOr(b.Out, io.Discard)
	errW := writerOr(b.Err, io.Discard)
	if err := runner.Run(ctx, "docker", args, out, errW); err != nil {
		return fmt.Errorf("docker buildx failed: %w", err)
	}
	return nil
}

func writerOr(w, fallback io.Writer) io.Writer {
	if w == nil {
		return fallback
	}
	return w
}

type execRunner struct{}

func (e *execRunner) LookPath(name string) error {
	_, err := exec.LookPath(name)
	return err
}

func (e *execRunner) Run(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}
