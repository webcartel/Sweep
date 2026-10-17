package cli

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
)

// execute runs the root command with the given arguments and returns
// everything it wrote to stdout and stderr.
func execute(t *testing.T, args ...string) (string, error) {
	t.Helper()

	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return out.String(), err
}

func TestVersionCommandReportsVersion(t *testing.T) {
	old := Version
	t.Cleanup(func() { Version = old })
	Version = "1.2.3-test"

	out, err := execute(t, "version")
	if err != nil {
		t.Fatalf("sweep version returned an error: %v", err)
	}

	const wantPrefix = "sweep 1.2.3-test (commit "
	if !strings.HasPrefix(out, wantPrefix) {
		t.Errorf("sweep version output = %q, want it to start with %q", out, wantPrefix)
	}
	if !strings.Contains(out, runtime.Version()) {
		t.Errorf("sweep version output = %q, want it to name the Go toolchain %q", out, runtime.Version())
	}
	if body := strings.TrimRight(out, "\n"); strings.Contains(body, "\n") {
		t.Errorf("sweep version output = %q, want a single line", out)
	}
}

func TestRootWithoutArgsPrintsHelp(t *testing.T) {
	out, err := execute(t)
	if err != nil {
		t.Fatalf("sweep without arguments returned an error: %v", err)
	}
	if !strings.Contains(out, "Usage:") {
		t.Errorf("sweep without arguments printed %q, want the help text", out)
	}
}
