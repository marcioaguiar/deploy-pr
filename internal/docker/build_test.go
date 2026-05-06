package docker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

type fakeRunner struct {
	lookPathErr error
	runErr      error

	gotName   string
	gotArgs   []string
	gotStdout string
	gotStderr string
}

func (f *fakeRunner) LookPath(string) error { return f.lookPathErr }

func (f *fakeRunner) Run(_ context.Context, name string, args []string, stdout, stderr io.Writer) error {
	f.gotName = name
	f.gotArgs = args
	if stdout != nil {
		_, _ = stdout.Write([]byte("build progress\n"))
	}
	if stderr != nil {
		_, _ = stderr.Write([]byte("warn\n"))
	}
	return f.runErr
}

func TestBuild_AssemblesExpectedArgs(t *testing.T) {
	r := &fakeRunner{}
	b := &Builder{Runner: r, Out: io.Discard, Err: io.Discard}
	err := b.Build(context.Background(), BuildOptions{
		ContextDir: ".",
		Dockerfile: "ops/Dockerfile",
		Tags: []string{
			"111111111111.dkr.ecr.us-east-1.amazonaws.com/myapp:pr-1-abc1234",
			"111111111111.dkr.ecr.us-east-1.amazonaws.com/myapp:pr-1-latest",
		},
		Push: true,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if r.gotName != "docker" {
		t.Errorf("cmd = %q", r.gotName)
	}
	want := []string{
		"buildx", "build", "--platform", "linux/amd64",
		"--file", "ops/Dockerfile",
		"--tag", "111111111111.dkr.ecr.us-east-1.amazonaws.com/myapp:pr-1-abc1234",
		"--tag", "111111111111.dkr.ecr.us-east-1.amazonaws.com/myapp:pr-1-latest",
		"--push", ".",
	}
	if !reflect.DeepEqual(r.gotArgs, want) {
		t.Errorf("args mismatch:\n got %v\nwant %v", r.gotArgs, want)
	}
}

func TestBuild_DefaultPlatform(t *testing.T) {
	r := &fakeRunner{}
	b := &Builder{Runner: r}
	_ = b.Build(context.Background(), BuildOptions{
		ContextDir: ".",
		Tags:       []string{"x:y"},
	})
	if !contains(r.gotArgs, "linux/amd64") {
		t.Errorf("default platform not applied: %v", r.gotArgs)
	}
}

func TestBuild_NoPushOmitsFlag(t *testing.T) {
	r := &fakeRunner{}
	b := &Builder{Runner: r}
	_ = b.Build(context.Background(), BuildOptions{
		ContextDir: ".",
		Tags:       []string{"x:y"},
	})
	if contains(r.gotArgs, "--push") {
		t.Errorf("--push should be omitted when Push=false: %v", r.gotArgs)
	}
}

func TestBuild_RequiresTags(t *testing.T) {
	b := &Builder{Runner: &fakeRunner{}}
	err := b.Build(context.Background(), BuildOptions{ContextDir: "."})
	if err == nil || !strings.Contains(err.Error(), "tag") {
		t.Errorf("expected tag-required error, got %v", err)
	}
}

func TestBuild_RequiresContextDir(t *testing.T) {
	b := &Builder{Runner: &fakeRunner{}}
	err := b.Build(context.Background(), BuildOptions{Tags: []string{"x:y"}})
	if err == nil || !strings.Contains(err.Error(), "context") {
		t.Errorf("expected context-required error, got %v", err)
	}
}

func TestBuild_DockerNotOnPath(t *testing.T) {
	b := &Builder{Runner: &fakeRunner{lookPathErr: errors.New("not found")}}
	err := b.Build(context.Background(), BuildOptions{
		ContextDir: ".", Tags: []string{"x:y"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "docker CLI not on PATH") {
		t.Errorf("error should be the friendly missing-docker message: %v", err)
	}
}

func TestBuild_RunFailureWraps(t *testing.T) {
	b := &Builder{Runner: &fakeRunner{runErr: errors.New("exit 1")}}
	err := b.Build(context.Background(), BuildOptions{
		ContextDir: ".", Tags: []string{"x:y"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "buildx") {
		t.Errorf("error should mention buildx: %v", err)
	}
}

func TestBuild_StreamsOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	b := &Builder{Runner: &fakeRunner{}, Out: &stdout, Err: &stderr}
	_ = b.Build(context.Background(), BuildOptions{
		ContextDir: ".", Tags: []string{"x:y"},
	})
	if !strings.Contains(stdout.String(), "build progress") {
		t.Errorf("stdout not propagated: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "warn") {
		t.Errorf("stderr not propagated: %q", stderr.String())
	}
}

func contains(s []string, want string) bool {
	for _, x := range s {
		if x == want {
			return true
		}
	}
	return false
}
