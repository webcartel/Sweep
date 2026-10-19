package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/platform/fake"
)

// cleanExec runs `sweep clean` with its arguments on the deps given and
// returns what it printed and how it ended.
func cleanExec(t *testing.T, d deps, args ...string) (string, string, error) {
	t.Helper()
	root := newRootCmd(d)
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append([]string{"clean"}, args...))
	err := root.Execute()
	return out.String(), errOut.String(), err
}

// cleanDeps goes like scanDeps and points the log of the machine at a real
// directory of the test's own: the file is written with the os of the
// process, so its place has to exist on this machine.
func cleanDeps(t *testing.T) deps {
	t.Helper()
	d := scanDeps(t)
	d.LogDir = t.TempDir()
	return d
}

// userRulesDir writes a rule file of the test's own into a directory of its
// own and returns it; the tests point --rules-dir at what it names.
func userRulesDir(t *testing.T, source string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "user.yaml")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return dir
}

// skipWarnings asserts that the stderr a throwaway scan holds nothing but the
// not_found skips of the built-in rules whose folders the fake machine does
// not have: the set grows, the fake tree does not have to grow with it.
func skipWarnings(t *testing.T, errOut string) {
	t.Helper()
	if errOut == "" {
		return
	}
	for _, line := range strings.Split(strings.TrimSuffix(errOut, "\n"), "\n") {
		if !strings.Contains(line, ": not_found") {
			t.Errorf("stderr holds more than an absent folder: %q", line)
		}
	}
}

func TestCleanWithoutATerminalWantsTheAgreementOnTheCommandLine(t *testing.T) {
	d := cleanDeps(t)

	out, errOut, err := cleanExec(t, d)
	if err == nil {
		t.Fatal("sweep clean without --yes ran with no terminal to ask")
	}
	if code := exitCode(t, err); code != ExitNoTerminal {
		t.Errorf("exit code = %d, want %d", code, ExitNoTerminal)
	}
	if !strings.Contains(errOut, "--yes") {
		t.Errorf("stderr = %q, want it to name the way out with --yes", errOut)
	}
	if out != "" {
		t.Errorf("stdout = %q, want nothing of a run", out)
	}
}

// logDir names the folder the log of a run goes to.
func logDir(t *testing.T, d deps) string {
	t.Helper()
	return filepath.Join(d.LogDir, "sweep", "logs")
}

// lastDocument answers the last JSON object the output holds: `sweep clean
// --json` prints the plan first and the answer of the run second.
func lastDocument(out string) string {
	at := strings.Index(out, `"started_at"`)
	if at < 0 {
		return out
	}
	start := strings.LastIndex(out[:at], "\n{")
	if start < 0 {
		return out
	}
	return out[start+1:]
}

func TestCleanRunsAndTellsTheTally(t *testing.T) {
	d := cleanDeps(t)

	out, errOut, err := cleanExec(t, d, "--yes")
	if err != nil {
		t.Fatalf("sweep clean --yes: %v", err)
	}
	skipWarnings(t, errOut)
	fsys := d.FS.(*fake.FS)
	if _, err := fsys.Lstat(`C:\Users\probe\AppData\Local\Temp\old.log`); !os.IsNotExist(err) {
		t.Errorf("the old log survived: %v", err)
	}
	if _, err := fsys.Lstat(`C:\Users\probe\AppData\Local\Temp\fresh.log`); err != nil {
		t.Error("the fresh log was taken away with the rest")
	}
	if !strings.Contains(out, "cleaned:") {
		t.Errorf("stdout = %q, want the summary of the run", out)
	}
	// The trash of the rules is the recycle bin: nothing went there.
	if trashed := fsys.Trashed(); len(trashed) != 0 {
		t.Errorf("Trashed() = %v, want nothing sent to the bin", trashed)
	}
}

func TestCleanDryRunTellsThePlanAndLeavesTheDisk(t *testing.T) {
	d := cleanDeps(t)

	out, _, err := cleanExec(t, d, "--dry-run")
	if err != nil {
		t.Fatalf("sweep clean --dry-run: %v", err)
	}
	fsys := d.FS.(*fake.FS)
	if removed := fsys.Removed(); len(removed) != 0 {
		t.Errorf("Removed() = %v, want nothing taken away", removed)
	}
	if !strings.Contains(out, `"dry_run": true`) {
		t.Errorf("stdout = %q, want the answer of the dry run", out)
	}
}

func TestCleanJSONIsTheAnswerOfTheRun(t *testing.T) {
	d := cleanDeps(t)

	out, _, err := cleanExec(t, d, "--yes", "--json")
	if err != nil {
		t.Fatalf("sweep clean --yes --json: %v", err)
	}
	var document struct {
		Schema   int    `json:"schema"`
		DryRun   bool   `json:"dry_run"`
		Finished string `json:"finished_at"`
		Summary  struct {
			Rules      int   `json:"rules"`
			Removed    int   `json:"removed"`
			BytesFreed int64 `json:"bytes_freed"`
		} `json:"summary"`
		Rules []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"rules"`
	}
	if err := json.Unmarshal([]byte(lastDocument(out)), &document); err != nil {
		t.Fatalf("the answer is not JSON: %v", err)
	}
	if document.Schema != 1 || document.DryRun || document.Finished == "" {
		t.Errorf("document = %+v", document)
	}
	if document.Summary.Rules == 0 || document.Summary.Removed == 0 {
		t.Errorf("summary = %+v, want the run in it", document.Summary)
	}
	for _, rule := range document.Rules {
		if rule.ID == "system.user-temp" && rule.Status != "done" {
			t.Errorf("status of %s = %s, want done", rule.ID, rule.Status)
		}
	}
}

func TestCleanNoTrashOnADangerousRuleIsAUsageError(t *testing.T) {
	d := cleanDeps(t)
	dir := userRulesDir(t,
		"schema: 1\nrules:\n  - id: user.precious\n    name: Precious\n"+
			"    category: user\n    risk: dangerous\n    trash: true\n"+
			"    targets:\n      - path: 'C:\\probe\\precious'\n        mode: contents\n")

	_, errOut, err := cleanExec(t, d, "--yes", "--rules-dir", dir,
		"--rule", "user.precious", "--no-trash")
	if err == nil {
		t.Fatal("sweep clean took the trash away from a dangerous rule")
	}
	if code := exitCode(t, err); code != ExitUsage {
		t.Errorf("exit code = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errOut, "user.precious") {
		t.Errorf("stderr = %q, want it to name the rule it refused", errOut)
	}
	fsys := d.FS.(*fake.FS)
	if removed := fsys.Removed(); len(removed) != 0 {
		t.Errorf("Removed() = %v, want nothing taken away", removed)
	}
}

func TestCleanKeepsADangerousRuleOutUntilItIsAskedFor(t *testing.T) {
	d := cleanDeps(t)
	dir := userRulesDir(t,
		"schema: 1\nrules:\n  - id: user.precious\n    name: Precious\n"+
			"    category: user\n    risk: dangerous\n    default_enabled: true\n    trash: true\n"+
			"    targets:\n      - path: '%TEMP%\\precious.log'\n        mode: self\n")
	fsys := d.FS.(*fake.FS)
	fsys.Add(platform.Info{
		Path:    `C:\Users\probe\AppData\Local\Temp\precious.log`,
		ModTime: time.Now(),
	})

	if _, _, err := cleanExec(t, d, "--yes", "--rules-dir", dir); err != nil {
		t.Fatalf("sweep clean --yes: %v", err)
	}
	for _, gone := range fsys.Removed() {
		if strings.Contains(gone, "precious.log") {
			t.Errorf("the dangerous rule ran without being asked for: %s", gone)
		}
	}
	if trashed := fsys.Trashed(); len(trashed) != 0 {
		t.Errorf("Trashed() = %v, want the dangerous data left alone", trashed)
	}
	if !oneOf(fsys.Removed(), `old.log`) {
		t.Error("the ordinary rules did not run")
	}

	fsys.Add(platform.Info{
		Path:    `C:\Users\probe\AppData\Local\Temp\precious.log`,
		ModTime: time.Now(),
	})
	if out2, errOut2, err := cleanExec(t, d, "--yes", "--rules-dir", dir, "--include-dangerous", "--json"); err != nil {
		t.Fatalf("sweep clean --yes --include-dangerous: %v (stderr %v)", err, errOut2)
	} else {
		if !strings.Contains(out2, "precious") {
			t.Errorf("the second run answered %q", out2)
		}
	}
	if !oneOf(fsys.Trashed(), `precious.log`) {
		t.Errorf("Trashed() = %v removed %v, want the dangerous data in the bin", fsys.Trashed(), fsys.Removed())
	}
}

// oneOf says whether any of the paths holds the tail named.
func oneOf(paths []string, tail string) bool {
	for _, path := range paths {
		if strings.Contains(path, tail) {
			return true
		}
	}
	return false
}

func TestCleanLogTellsWhatLeft(t *testing.T) {
	d := cleanDeps(t)
	want := logDir(t, d)

	if _, _, err := cleanExec(t, d, "--yes"); err != nil {
		t.Fatalf("sweep clean --yes: %v", err)
	}
	entries, err := os.ReadDir(want)
	if err != nil || len(entries) == 0 {
		t.Fatalf("no log in %s: %v", want, err)
	}
	if len(entries) != 1 || !strings.HasSuffix(entries[0].Name(), ".jsonl") {
		t.Fatalf("the logs of %s hold %v, want one jsonl file of today", want, entries)
	}
	data, err := os.ReadFile(filepath.Join(want, entries[0].Name()))
	if err != nil {
		t.Fatalf("read the log: %v", err)
	}
	if !strings.Contains(string(data), "old.log") {
		t.Errorf("the log says %q, want it to name what left", data)
	}
}

func TestCleanThatCannotLogDoesNotStart(t *testing.T) {
	d := cleanDeps(t)
	// The place a log would go to is taken by a file: nothing can be
	// written under it, so nothing runs.
	taken := filepath.Join(t.TempDir(), "taken")
	if err := os.WriteFile(taken, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write %s: %v", taken, err)
	}
	d.LogDir = taken

	_, _, err := cleanExec(t, d, "--yes")
	if err == nil {
		t.Fatal("sweep clean ran with the log gone")
	}
	if code := exitCode(t, err); code != ExitInternal {
		t.Errorf("exit code = %d, want %d", code, ExitInternal)
	}
}
