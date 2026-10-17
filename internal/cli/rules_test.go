package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeRuleFile writes a rule file of the test's own and returns its path.
func writeRuleFile(t *testing.T, name, source string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// brokenRuleFile writes a rule file with one field the format does not have: the
// finding lands on line 7, column 5.
func brokenRuleFile(t *testing.T) string {
	t.Helper()

	return writeRuleFile(t, "broken.yaml",
		"schema: 1\nrules:\n  - id: user.broken\n    name: Broken\n    category: user\n"+
			"    risk: safe\n    nope: 1\n    targets:\n      - path: 'C:\\probe'\n")
}

// exitCode returns the code the process would leave with for an error.
func exitCode(t *testing.T, err error) int {
	t.Helper()

	var exit *ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("error = %v (%T), want an ExitError carrying the code", err, err)
	}
	return exit.Code
}

// The findings of a checker belong on the error stream, where cobra writes them
// once; stdout stays what the caller asked for.
func TestRulesValidateReportsPositionOnBrokenFile(t *testing.T) {
	path := brokenRuleFile(t)

	out, errOut, err := execute(t, "rules", "validate", path)
	if err == nil {
		t.Fatal("sweep rules validate accepted a file the format does not allow")
	}
	if code := exitCode(t, err); code != ExitUsage {
		t.Errorf("exit code = %d, want %d", code, ExitUsage)
	}
	if want := path + ":7:5:"; !strings.Contains(errOut, want) {
		t.Errorf("sweep rules validate wrote %q to stderr, want it to hold %q", errOut, want)
	}
	if out != "" {
		t.Errorf("sweep rules validate printed %q to stdout, want nothing", out)
	}
}

// A directory stands for the rule files it holds, so that pointing the command
// at a folder does what the reader expects, and a file that is not a rule file
// is left alone.
func TestRulesValidateReadsADirectory(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())

	broken := brokenRuleFile(t)
	dir := filepath.Dir(broken)

	good := filepath.Join(dir, "good.yaml")
	if err := os.WriteFile(good, []byte("schema: 1\nrules: []\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", good, err)
	}
	notes := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(notes, []byte("not a rule file\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", notes, err)
	}

	_, errOut, err := execute(t, "rules", "validate", dir)
	if err == nil {
		t.Fatal("sweep rules validate accepted a directory that holds a broken rule file")
	}
	if want := broken + ":7:5:"; !strings.Contains(errOut, want) {
		t.Errorf("sweep rules validate wrote %q, want it to hold %q", errOut, want)
	}
	if strings.Contains(errOut, good) {
		t.Errorf("sweep rules validate wrote %q, want the good file left alone", errOut)
	}
	if strings.Contains(errOut, notes) {
		t.Errorf("sweep rules validate wrote %q, want a file that is not a rule file left alone", errOut)
	}
}

// A path that cannot be read is named as the system named it, once: the system
// already says which path it could not open.
func TestRulesValidateNamesAMissingFileOnce(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())

	missing := filepath.Join(t.TempDir(), "nope.yaml")
	_, errOut, err := execute(t, "rules", "validate", missing)
	if err == nil {
		t.Fatal("sweep rules validate accepted a file that is not there")
	}
	if code := exitCode(t, err); code != ExitUsage {
		t.Errorf("exit code = %d, want %d", code, ExitUsage)
	}
	if got := strings.Count(errOut, missing); got != 1 {
		t.Errorf("sweep rules validate wrote %q, want the path named once, not %d times", errOut, got)
	}
}

// Without paths the command checks everything sweep loads, and what it finds is
// the error of the command like any other finding.
func TestRulesValidateWithoutPathsReportsABrokenRuleFile(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())

	dir := t.TempDir()
	broken := filepath.Join(dir, "broken.yaml")
	if err := os.WriteFile(broken, []byte("schema: 1\nrules:\n  - id: user.broken\n    name: Broken\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", broken, err)
	}

	out, errOut, err := execute(t, "rules", "validate", "--rules-dir", dir)
	if err == nil {
		t.Fatal("sweep rules validate accepted a broken rule file of an extra directory")
	}
	if code := exitCode(t, err); code != ExitUsage {
		t.Errorf("exit code = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errOut, broken) {
		t.Errorf("sweep rules validate wrote %q to stderr, want it to name %s", errOut, broken)
	}
	if out != "" {
		t.Errorf("sweep rules validate printed %q to stdout, want nothing", out)
	}
}

func TestRulesValidateIsQuietOnGoodFiles(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())

	path := writeRuleFile(t, "good.yaml",
		"schema: 1\nrules:\n  - id: user.probe\n    name: Probe\n    category: user\n"+
			"    risk: safe\n    targets:\n      - path: 'C:\\probe'\n")

	out, errOut, err := execute(t, "rules", "validate", path)
	if err != nil {
		t.Fatalf("sweep rules validate on a good file: %v", err)
	}
	if out != "" {
		t.Errorf("sweep rules validate printed %q to stdout, want nothing", out)
	}
	if errOut != "" {
		t.Errorf("sweep rules validate wrote %q to stderr, want nothing", errOut)
	}
}

func TestRulesValidateWithoutPathsReadsEverySource(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())

	out, errOut, err := execute(t, "rules", "validate")
	if err != nil {
		t.Fatalf("sweep rules validate on the built-in rules: %v", err)
	}
	if out != "" {
		t.Errorf("sweep rules validate printed %q to stdout, want nothing", out)
	}
	if errOut != "" {
		t.Errorf("sweep rules validate wrote %q to stderr, want nothing on a machine with no rules of its own", errOut)
	}
}

func TestRulesShowPrintsMergedRule(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())

	out, errOut, err := execute(t, "rules", "show", "system.user-temp")
	if err != nil {
		t.Fatalf("sweep rules show: %v", err)
	}
	if errOut != "" {
		t.Errorf("sweep rules show wrote %q to stderr, want nothing", errOut)
	}
	if !strings.Contains(out, "system.user-temp") {
		t.Errorf("sweep rules show printed %q, want it to name the rule", out)
	}
	if !strings.Contains(out, "source: builtin") {
		t.Errorf("sweep rules show printed %q, want it to say where the rule came from", out)
	}

	temp, ok := os.LookupEnv("TEMP")
	if !ok {
		t.Skip("TEMP is not set on this machine, so there is nothing to expand")
	}
	if !strings.Contains(out, temp) {
		t.Errorf("sweep rules show printed %q, want the path expanded to %q", out, temp)
	}
	if strings.Contains(out, "%TEMP%") {
		t.Errorf("sweep rules show printed %q, want the variable expanded", out)
	}
}

func TestRulesShowUnknownIDIsAnError(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())

	_, _, err := execute(t, "rules", "show", "nope.nope")
	if err == nil {
		t.Fatal("sweep rules show accepted an id no source defines")
	}
	if code := exitCode(t, err); code != ExitUsage {
		t.Errorf("exit code = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(err.Error(), "nope.nope") {
		t.Errorf("error = %v, want it to name the id", err)
	}
}

func TestRulesShowWithoutAnIDIsAUsageError(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())

	_, _, err := execute(t, "rules", "show")
	if err == nil {
		t.Fatal("sweep rules show accepted a call without an id")
	}
	if code := exitCode(t, err); code != ExitUsage {
		t.Errorf("exit code = %d, want %d", code, ExitUsage)
	}
}
