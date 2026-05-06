package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// runCmd builds a fresh root command, runs it with the given args, and returns
// captured stdout, stderr, and the error. It clears DEPLOY_PR_* env vars so a
// developer's shell does not leak into assertions.
func runCmd(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	clearTestEnv(t)
	chdirTemp(t)

	var out, errBuf bytes.Buffer
	root := NewRootCmd(&out, &errBuf)
	root.SetArgs(args)
	err = root.Execute()
	return out.String(), errBuf.String(), err
}

func TestRoot_HelpListsAllSubcommands(t *testing.T) {
	stdout, _, err := runCmd(t, "--help")
	if err != nil {
		t.Fatalf("--help returned error: %v", err)
	}
	for _, sub := range []string{"up", "down", "list", "version"} {
		if !strings.Contains(stdout, sub) {
			t.Errorf("--help output missing subcommand %q\n%s", sub, stdout)
		}
	}
}

func TestVersion_PrintsTokens(t *testing.T) {
	stdout, _, err := runCmd(t, "version")
	if err != nil {
		t.Fatalf("version returned error: %v", err)
	}
	if !strings.Contains(stdout, "deploy-pr ") {
		t.Errorf("version output unexpected: %q", stdout)
	}
}

func TestUp_StubReturnsError(t *testing.T) {
	// up requires a PR arg; provide one. The stub must return a non-nil error.
	_, _, err := runCmd(t, "up", "1")
	if err == nil {
		t.Fatal("expected stub error from up")
	}
}

func TestUp_RequiresPRArgument(t *testing.T) {
	_, _, err := runCmd(t, "up")
	if err == nil {
		t.Fatal("expected error when up is called with no args")
	}
}

func TestExitCode_UserErrorIsOne(t *testing.T) {
	if got := exitCodeFor(userErr(errors.New("bad"))); got != 1 {
		t.Errorf("userErr exit code = %d, want 1", got)
	}
}

func TestExitCode_InfraErrorIsTwo(t *testing.T) {
	if got := exitCodeFor(infraErr(errors.New("aws down"))); got != 2 {
		t.Errorf("infraErr exit code = %d, want 2", got)
	}
}

func TestExitCode_NilIsZero(t *testing.T) {
	if got := exitCodeFor(nil); got != 0 {
		t.Errorf("nil exit code = %d, want 0", got)
	}
}

func TestLogger_JSONFormatIsParsable(t *testing.T) {
	var buf bytes.Buffer
	logger := newLogger(&buf, "json", false)
	logger.Info("hello", "key", "value")

	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("no log output")
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("log line is not valid JSON: %v\nline: %s", err, line)
	}
	if got["msg"] != "hello" || got["key"] != "value" {
		t.Errorf("unexpected JSON content: %v", got)
	}
}

func TestLogger_TextFormatIsHumanReadable(t *testing.T) {
	var buf bytes.Buffer
	logger := newLogger(&buf, "text", false)
	logger.Info("hello", "key", "value")

	line := buf.String()
	if !strings.Contains(line, "hello") || !strings.Contains(line, "key=value") {
		t.Errorf("text log missing tokens: %q", line)
	}
}

func TestLogger_VerboseEnablesDebug(t *testing.T) {
	var buf bytes.Buffer
	logger := newLogger(&buf, "text", true)
	logger.Debug("debugmsg")
	if !strings.Contains(buf.String(), "debugmsg") {
		t.Errorf("debug message not emitted with verbose=true: %q", buf.String())
	}

	buf.Reset()
	logger = newLogger(&buf, "text", false)
	logger.Debug("hidden")
	if strings.Contains(buf.String(), "hidden") {
		t.Errorf("debug message leaked at info level: %q", buf.String())
	}
}
