package cli

import (
	"bytes"
	"runtime"
	"strings"
	"testing"

	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/platform/fake"
	"github.com/webcartel/sweep/internal/tui"
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

func TestRootWithoutArgsRunsTUI(t *testing.T) {
	env := fake.NewEnv()
	env.Set("APPDATA", t.TempDir())
	env.Set("LOCALAPPDATA", t.TempDir())
	env.Set("TEMP", t.TempDir())
	var got tui.Deps
	var called bool
	d := deps{
		FS:     mustFS(t),
		Env:    env,
		System: fake.NewSystem(),
		Cwd:    t.TempDir(),
		TUI:    func(u tui.Deps) error { called = true; got = u; return nil },
	}
	root := newRootCmd(d)
	root.SetArgs([]string{"--ascii"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sweep without arguments returned %v", err)
	}
	if !called {
		t.Fatal("sweep without arguments did not open the TUI")
	}
	if !got.AsciiFlag {
		t.Fatal("the say of --ascii did not reach the TUI")
	}
	if got.Scanner == nil || got.Executor == nil || got.System == nil {
		t.Fatal("the TUI went without the cores of the run")
	}
	if len(got.Profiles) == 0 {
		t.Fatal("the TUI went without the profiles of the rules")
	}
	if got.RulesFor == nil {
		t.Fatal("the TUI went without the rules of a profile")
	}
	rules, err := got.RulesFor("default")
	if err != nil {
		t.Fatalf("the rules of the default profile: %v", err)
	}
	if len(rules) == 0 {
		t.Fatal("the default profile holds no rule")
	}
	if got.LogDir == "" {
		t.Fatal("the TUI went nowhere to keep its log")
	}
}

// mustFS gives the real filesystem of the machine, once.
func mustFS(t *testing.T) platform.FS {
	t.Helper()
	fsys, err := platform.NewFS()
	if err != nil {
		t.Fatalf("the filesystem failed: %v", err)
	}
	return fsys
}

// mustFS gives the real filesystem of the machine, once.
