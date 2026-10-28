package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/platform/fake"
)

// localOf answers %LOCALAPPDATA% of the machine the deps read.
func localOf(t *testing.T, d deps) string {
	t.Helper()
	value, ok := d.Env.Get("LOCALAPPDATA")
	if !ok {
		t.Fatal("%LOCALAPPDATA% is not set in the test machine")
	}
	return value
}

// registryMachine is a CLI over a registry and a disk in memory.
func registryMachine(t *testing.T) (deps, *fake.Registry, *fake.FS) {
	t.Helper()
	reg := fake.NewRegistry()
	fsys := fake.NewFS()
	fsys.SetDrive(`C:\`, platform.DriveFixed)
	env := fake.NewEnv()
	env.Set("LOCALAPPDATA", t.TempDir())
	d := deps{FS: fsys, Env: env, System: fake.NewSystem(), Registry: reg,
		Cwd: `C:\Users\probe`, LogDir: t.TempDir()}
	return d, reg, fsys
}

// runRegistry runs `sweep registry ...` on the deps given.
func runRegistry(t *testing.T, d deps, args ...string) (string, string, error) {
	t.Helper()
	cmd := newRootCmd(d, pinEnglish(append([]string{"registry"}, args...)))
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(append([]string{"registry"}, args...))
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

// One dead Run value reaches the table and the JSON document.
func TestRegistryScanPrints(t *testing.T) {
	d, reg, _ := registryMachine(t)
	reg.AddValue(`HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Run`,
		"Gone", platform.Value{Data: `C:\Gone\app.exe`, Kind: platform.ValueString})

	out, errOut, err := runRegistry(t, d, "scan")
	if err != nil {
		t.Fatalf("registry scan: %v", err)
	}
	if errOut != "" {
		t.Errorf("stderr = %q, want nothing", errOut)
	}
	for _, want := range []string{"KIND", "NAME", "run", "Gone", "program not found"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the table does not say %q: %q", want, out)
		}
	}

	out, _, err = runRegistry(t, d, "scan", "--json")
	if err != nil {
		t.Fatalf("registry scan --json: %v", err)
	}
	var answer registryScanAnswer
	if err := json.Unmarshal([]byte(out), &answer); err != nil {
		t.Fatalf("%q is not one document: %v", out, err)
	}
	if answer.Schema != 1 || len(answer.Findings) != 1 {
		t.Fatalf("answer = %+v", answer)
	}
	if answer.Findings[0].Kind != "run" || answer.Findings[0].Name != "Gone" {
		t.Fatalf("finding = %+v", answer.Findings[0])
	}
}

// The cleaner refuses an empty choice, asks without --yes, and exports
// before it deletes.
func TestRegistryCleanNeedsAChoiceAndConsent(t *testing.T) {
	d, reg, _ := registryMachine(t)
	run := `HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Run`
	reg.AddValue(run, "Gone", platform.Value{Data: `C:\Gone\app.exe`, Kind: platform.ValueString})

	if _, _, err := runRegistry(t, d, "clean"); err == nil {
		t.Fatal("clean ran without a choice")
	} else if code := exitCode(t, err); code != ExitUsage {
		t.Fatalf("exit code = %d, want %d", code, ExitUsage)
	}

	_, _, err := runRegistry(t, d, "clean", "--select", "run:"+run+":Gone")
	if code := exitCode(t, err); code != ExitNoTerminal {
		t.Fatalf("exit code = %d, want %d", code, ExitNoTerminal)
	}

	out, _, err := runRegistry(t, d, "clean", "--select", "run:"+run+":Gone", "--yes")
	if err != nil {
		t.Fatalf("clean --yes: %v", err)
	}
	if !strings.Contains(out, "removed 1") || !strings.Contains(out, "backup ") {
		t.Fatalf("output = %q", out)
	}
	for _, op := range reg.Operations {
		if strings.HasPrefix(op, "delete") {
			continue
		}
		if !strings.HasPrefix(op, "export") {
			t.Fatalf("unexpected operation %q", op)
		}
	}
	backups := filepath.Join(localOf(t, d), "sweep", "registry-backups")
	entries, err := os.ReadDir(backups)
	if err != nil || len(entries) != 1 {
		t.Fatalf("backups = %v, %v", entries, err)
	}
	if _, err := os.Stat(filepath.Join(backups, entries[0].Name(), "manifest.json")); err != nil {
		t.Fatalf("no manifest: %v", err)
	}
}

// The clean JSON is one document with the backup directory.
func TestRegistryCleanJSON(t *testing.T) {
	d, reg, _ := registryMachine(t)
	run := `HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Run`
	reg.AddValue(run, "Gone", platform.Value{Data: `C:\Gone\app.exe`})

	out, _, err := runRegistry(t, d, "clean", "--select", "run:"+run+":Gone", "--yes", "--json")
	if err != nil {
		t.Fatalf("clean --json: %v", err)
	}
	var answer registryCleanAnswer
	if err := json.Unmarshal([]byte(out), &answer); err != nil {
		t.Fatalf("%q is not one document: %v", out, err)
	}
	if answer.Schema != 1 || answer.Removed != 1 || answer.BackupDir == "" {
		t.Fatalf("answer = %+v", answer)
	}
}

// The dry run touches nothing and needs no choice.
func TestRegistryCleanDryRun(t *testing.T) {
	d, reg, _ := registryMachine(t)
	run := `HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Run`
	reg.AddValue(run, "Gone", platform.Value{Data: `C:\Gone\app.exe`})

	out, _, err := runRegistry(t, d, "clean", "--dry-run")
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !strings.Contains(out, "planned") && !strings.Contains(out, "Gone") {
		t.Fatalf("the dry run hides the plan: %q", out)
	}
	if len(reg.Operations) != 0 {
		t.Fatalf("the dry run touched the registry: %v", reg.Operations)
	}
}

// The command line declares the cleaner; the plan fills it in.
func TestRegistryCommandIsDeclared(t *testing.T) {
	cmd := newRootCmd(deps{}, nil)
	found := false
	for _, child := range cmd.Commands() {
		if child.Name() == "registry" {
			found = true
		}
	}
	if !found {
		t.Fatal("the root command has no registry branch")
	}
}
