package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/platform/fake"
)

// registryMachine is a CLI over a registry and a disk in memory.
func registryMachine(t *testing.T) (deps, *fake.Registry, *fake.FS) {
	t.Helper()
	reg := fake.NewRegistry()
	fsys := fake.NewFS()
	fsys.SetDrive(`C:\`, platform.DriveFixed)
	env := fake.NewEnv()
	env.Set("LOCALAPPDATA", `C:\Users\probe\AppData\Local`)
	d := deps{FS: fsys, Env: env, System: fake.NewSystem(), Registry: reg,
		Cwd: `C:\Users\probe`, LogDir: t.TempDir()}
	return d, reg, fsys
}

// runRegistry runs `sweep registry ...` on the deps given.
func runRegistry(t *testing.T, d deps, args ...string) (string, string, error) {
	t.Helper()
	cmd := newRootCmd(d)
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

// The command line declares the cleaner; the plan fills it in.
func TestRegistryCommandIsDeclared(t *testing.T) {
	cmd := newRootCmd(deps{})
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
