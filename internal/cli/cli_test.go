package cli

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
)

// execute runs the root command with the given arguments and returns what it
// wrote to stdout, what it wrote to stderr, and the error it returned. The two
// streams are kept apart on purpose: a command that sends its answer to stderr
// breaks every script that reads stdout.
func execute(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	cmd := NewRootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)

	err = cmd.Execute()
	return out.String(), errOut.String(), err
}

func TestVersionCommandReportsVersion(t *testing.T) {
	old := Version
	t.Cleanup(func() { Version = old })
	Version = "1.2.3-test"

	out, errOut, err := execute(t, "version")
	if err != nil {
		t.Fatalf("sweep version returned an error: %v", err)
	}
	if errOut != "" {
		t.Errorf("sweep version wrote %q to stderr, want nothing", errOut)
	}

	const wantPrefix = "sweep 1.2.3-test (commit "
	line := strings.TrimSuffix(out, "\n")
	if !strings.HasPrefix(line, wantPrefix) {
		t.Errorf("sweep version output = %q, want it to start with %q", line, wantPrefix)
	}
	if !strings.HasSuffix(line, ", "+runtime.Version()+")") {
		t.Errorf("sweep version output = %q, want it to end with the Go toolchain %q", line, runtime.Version())
	}
	if body := strings.TrimRight(out, "\n"); strings.Contains(body, "\n") {
		t.Errorf("sweep version output = %q, want a single line", out)
	}
}

func TestUnknownFlagIsAUsageError(t *testing.T) {
	_, _, err := execute(t, "list", "--nope")
	if err == nil {
		t.Fatal("sweep list accepted a flag it does not have")
	}
	if code := exitCode(t, err); code != ExitUsage {
		t.Errorf("exit code = %d, want %d", code, ExitUsage)
	}
}

func TestRootWithoutArgsPrintsHelp(t *testing.T) {
	out, errOut, err := execute(t)
	if err != nil {
		t.Fatalf("sweep without arguments returned an error: %v", err)
	}
	if !strings.Contains(out, "Usage:") {
		t.Errorf("sweep without arguments printed %q to stdout, want the help text", out)
	}
	if errOut != "" {
		t.Errorf("sweep without arguments wrote %q to stderr, want nothing", errOut)
	}
}
