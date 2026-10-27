package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/platform/fake"
)

// cleanFixture is a registry with one Run value and one user key to delete.
func cleanFixture() (*fake.Registry, []Finding, string) {
	run := `HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Run`
	app := `HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\App Paths\gone.exe`
	reg := fake.NewRegistry()
	reg.AddValue(run, "Gone", platform.Value{Data: `C:\Gone\app.exe`, Kind: platform.ValueString})
	reg.AddValue(app, "", platform.Value{Data: `C:\Gone\app.exe`, Kind: platform.ValueString})
	findings := []Finding{
		{Kind: KindRun, ID: "run|" + run + "|gone", Name: "Gone", Key: run, Value: "Gone",
			Path: `C:\Gone\app.exe`, Reason: "program not found", DeleteValue: true},
		{Kind: KindAppPath, ID: "apppath|" + app + "|", Name: "gone.exe", Key: app,
			Path: `C:\Gone\app.exe`, Reason: "program not found"},
	}
	return reg, findings, filepath.Join(os.TempDir(), "sweep-backups-test")
}

// A dry run changes nothing and writes no backup.
func TestCleanDryRun(t *testing.T) {
	reg, findings, root := cleanFixture()
	result, err := Clean(reg, findings, Options{DryRun: true, BackupRoot: root})
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if len(reg.Operations) != 0 {
		t.Fatalf("the dry run touched the registry: %v", reg.Operations)
	}
	if result.BackupDir != "" {
		t.Fatalf("the dry run wrote %q", result.BackupDir)
	}
	if len(result.Entries) != 2 || result.Entries[0].Action != "planned" {
		t.Fatalf("entries = %+v", result.Entries)
	}
}

// A failed export leaves the key alone and says why.
func TestCleanExportFailureKeepsTheKey(t *testing.T) {
	reg, findings, root := cleanFixture()
	run := findings[0].Key
	reg.FailExport = map[string]error{run: os.ErrPermission}
	result, err := Clean(reg, findings[:1], Options{BackupRoot: root, Elevated: true})
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	for _, op := range reg.Operations {
		if strings.HasPrefix(op, "delete") {
			t.Fatalf("the key was deleted without a backup: %v", reg.Operations)
		}
	}
	if result.Failed != 1 || !strings.Contains(result.Entries[0].Note, "backup failed") {
		t.Fatalf("result = %+v", result)
	}
}

// HKLM needs elevation; without it the finding is skipped.
func TestCleanNeedsAdmin(t *testing.T) {
	reg := fake.NewRegistry()
	key := `HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Run`
	reg.AddValue(key, "Gone", platform.Value{Data: `C:\Gone\app.exe`})
	finding := Finding{Kind: KindRun, ID: "x", Name: "Gone", Key: key, Value: "Gone",
		Path: `C:\Gone\app.exe`, Reason: "program not found", Admin: true, DeleteValue: true}
	result, err := Clean(reg, []Finding{finding}, Options{BackupRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if result.Skipped != 1 || result.Entries[0].Note != "needs_admin" {
		t.Fatalf("result = %+v", result)
	}
	if len(reg.Operations) != 0 {
		t.Fatalf("the elevated key was touched: %v", reg.Operations)
	}
}

// A key with subkeys is skipped.
func TestCleanSkipsSubkeys(t *testing.T) {
	reg, findings, root := cleanFixture()
	findings[1].HasSubkeys = true
	result, err := Clean(reg, findings[1:], Options{BackupRoot: root, Elevated: true})
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if result.Skipped != 1 || result.Entries[0].Note != "has subkeys" {
		t.Fatalf("result = %+v", result)
	}
}

// The successful path exports before it deletes, writes the manifest, and
// counts both parts.
func TestCleanExportsBeforeDeleting(t *testing.T) {
	reg, findings, root := cleanFixture()
	dir := t.TempDir()
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	var logged []string
	result, err := Clean(reg, findings, Options{
		BackupRoot: dir, Elevated: true, Now: func() time.Time { return now },
		Log: func(action, path, note string) { logged = append(logged, action) },
	})
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if result.Removed != 2 || result.Failed != 0 {
		t.Fatalf("result = %+v", result)
	}
	if result.BackupDir != filepath.Join(dir, "20261028-120000") {
		t.Fatalf("backup dir = %q", result.BackupDir)
	}
	ops := strings.Join(reg.Operations, "\n")
	if strings.Index(ops, "delete") < strings.Index(ops, "export") {
		t.Fatalf("a delete came before an export:\n%s", ops)
	}
	manifest, err := os.ReadFile(filepath.Join(result.BackupDir, "manifest.json"))
	if err != nil || !strings.Contains(string(manifest), "backup-01.reg") {
		t.Fatalf("manifest = %q, %v", manifest, err)
	}
	if len(logged) != 2 || logged[0] != "registry_delete" {
		t.Fatalf("logged = %v", logged)
	}
	_ = root
}

// The selector names a finding by kind, key and value.
func TestSelect(t *testing.T) {
	run := `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
	findings := []Finding{
		{Kind: KindRun, Key: run, Value: "Gone"},
		{Kind: KindRun, Key: run, Value: "Other"},
	}
	selected, missing := Select(findings, []string{"run:" + run + ":Gone"})
	if len(selected) != 1 || selected[0].Value != "Gone" || len(missing) != 0 {
		t.Fatalf("selected = %+v missing = %v", selected, missing)
	}
	_, missing = Select(findings, []string{"run:" + run + ":Nope"})
	if len(missing) != 1 {
		t.Fatalf("missing = %v", missing)
	}
}
