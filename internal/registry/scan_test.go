package registry

import (
	"strings"
	"testing"

	"github.com/webcartel/sweep/internal/i18n"
	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/platform/fake"
)

// scanMachine is a registry and a disk with a local C: and one live program.
func scanMachine() (*fake.Registry, *fake.FS, platform.Env) {
	reg := fake.NewRegistry()
	fsys := fake.NewFS()
	fsys.SetDrive(`C:\`, platform.DriveFixed)
	fsys.Add(platform.Info{Path: `C:\Live\app.exe`})
	fsys.Add(platform.Info{Path: `C:\Program Files\App\app.exe`})
	env := fake.NewEnv()
	env.Set("LOCALAPPDATA", `C:\Users\probe\AppData\Local`)
	return reg, fsys, env
}

// scanRun scans and answers the findings of the run branch.
func scanRun(t *testing.T, reg *fake.Registry, fsys *fake.FS, env platform.Env) ([]Finding, []string) {
	t.Helper()
	findings, notes, err := Scan(reg, fsys, env)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	return findings, notes
}

const runKey = `HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Run`

// A Run value whose program is gone is a finding; a live one is not.
func TestRunFindings(t *testing.T) {
	reg, fsys, env := scanMachine()
	reg.AddValue(runKey, "Gone", platform.Value{Data: `"C:\Gone\app.exe" /silent`, Kind: platform.ValueString})
	reg.AddValue(runKey, "Live", platform.Value{Data: `C:\Live\app.exe`, Kind: platform.ValueString})
	reg.AddValue(runKey, "Spaced", platform.Value{Data: `C:\Program Files\App\app.exe -tray`, Kind: platform.ValueString})
	reg.AddValue(runKey, "VarGone", platform.Value{Data: `%LOCALAPPDATA%\Gone\app.exe`, Kind: platform.ValueExpandString})
	reg.AddValue(runKey, "Unknown", platform.Value{Data: `%NOPE%\app.exe`, Kind: platform.ValueString})

	findings, notes := scanRun(t, reg, fsys, env)
	if len(findings) != 2 {
		t.Fatalf("findings = %+v, want Gone and VarGone", findings)
	}
	for _, finding := range findings {
		if finding.Kind != KindRun || !finding.DeleteValue {
			t.Errorf("finding = %+v, want a Run value", finding)
		}
		if finding.Reason == "" {
			t.Error("a finding must explain itself")
		}
	}
	if !strings.Contains(strings.Join(notes, "\n"), "unknown variable %NOPE%") {
		t.Errorf("notes = %v, want the unknown variable", notes)
	}
}

// A program on a removable or network drive is left alone.
func TestRunOnRemovableDiskIsLeftAlone(t *testing.T) {
	reg, fsys, env := scanMachine()
	fsys.SetDrive(`E:\`, platform.DriveRemovable)
	reg.AddValue(runKey, "Stick", platform.Value{Data: `E:\Apps\app.exe`, Kind: platform.ValueString})

	findings, notes := scanRun(t, reg, fsys, env)
	if len(findings) != 0 {
		t.Fatalf("findings = %+v, want none", findings)
	}
	if !strings.Contains(strings.Join(notes, "\n"), "not on a local disk") {
		t.Errorf("notes = %v, want the local disk note", notes)
	}
}

// An uninstall key is a finding when its command is gone, and is skipped
// when Windows or an update owns it.
func TestUninstallFindings(t *testing.T) {
	parent := `HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Uninstall`
	reg, fsys, env := scanMachine()
	reg.AddValue(parent+`\Dead`, "DisplayName", platform.Value{Data: "Dead App"})
	reg.AddValue(parent+`\Dead`, "UninstallString", platform.Value{Data: `"C:\Gone\unins000.exe"`})
	reg.AddValue(parent+`\Live`, "DisplayName", platform.Value{Data: "Live App"})
	reg.AddValue(parent+`\Live`, "UninstallString", platform.Value{Data: `C:\Live\app.exe /uninstall`})
	reg.AddValue(parent+`\MSI`, "DisplayName", platform.Value{Data: "MSI App"})
	reg.AddValue(parent+`\MSI`, "WindowsInstaller", platform.Value{Data: "1", Kind: platform.ValueDWord})
	reg.AddValue(parent+`\MSI`, "UninstallString", platform.Value{Data: `C:\Gone\msi.exe`})
	reg.AddValue(parent+`\Update`, "DisplayName", platform.Value{Data: "Update"})
	reg.AddValue(parent+`\Update`, "ParentKeyName", platform.Value{Data: "Something"})
	reg.AddValue(parent+`\Update`, "UninstallString", platform.Value{Data: `C:\Gone\update.exe`})
	reg.AddValue(parent+`\System`, "DisplayName", platform.Value{Data: "Component"})
	reg.AddValue(parent+`\System`, "SystemComponent", platform.Value{Data: "1", Kind: platform.ValueDWord})
	reg.AddValue(parent+`\System`, "UninstallString", platform.Value{Data: `C:\Gone\component.exe`})
	reg.AddValue(parent+`\MsiCmd`, "DisplayName", platform.Value{Data: "MsiCmd"})
	reg.AddValue(parent+`\MsiCmd`, "UninstallString", platform.Value{Data: `MsiExec.exe /X{123}`})

	findings, _ := scanRun(t, reg, fsys, env)
	if len(findings) != 1 {
		t.Fatalf("findings = %+v, want only the dead uninstall", findings)
	}
	if findings[0].Kind != KindUninstall || findings[0].Name != "Dead App" {
		t.Fatalf("finding = %+v", findings[0])
	}
	if !strings.Contains(findings[0].Reason, "uninstall command not found") {
		t.Errorf("reason = %q", findings[0].Reason)
	}
}

// An App Paths key whose program is gone is a finding; subkeys mark it
// undeletable whole.
func TestAppPathFindings(t *testing.T) {
	parent := `HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\App Paths`
	reg, fsys, env := scanMachine()
	reg.AddValue(parent+`\gone.exe`, "", platform.Value{Data: `C:\Gone\app.exe`})
	reg.AddValue(parent+`\live.exe`, "", platform.Value{Data: `C:\Live\app.exe`})
	reg.AddKey(parent + `\gone.exe\Nested`)

	findings, _ := scanRun(t, reg, fsys, env)
	if len(findings) != 1 {
		t.Fatalf("findings = %+v, want the gone App Paths key", findings)
	}
	if findings[0].Kind != KindAppPath || !findings[0].HasSubkeys {
		t.Fatalf("finding = %+v, want a key with subkeys", findings[0])
	}
}

// Only the three branches pass the guard.
func TestGuarded(t *testing.T) {
	allowed := []string{
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
		`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\X`,
		`HKEY_LOCAL_MACHINE\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\App Paths\a.exe`,
	}
	for _, key := range allowed {
		if !Guarded(key) {
			t.Errorf("Guarded(%q) = false, want true", key)
		}
	}
	refused := []string{
		`HKLM\SYSTEM\CurrentControlSet\Services`,
		`HKLM\SOFTWARE\Classes\.txt`,
		`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion`,
		`HKCU\Software`,
	}
	for _, key := range refused {
		if Guarded(key) {
			t.Errorf("Guarded(%q) = true, want false", key)
		}
	}
}

// A note the user reads follows the language of the interface, and the
// English text is still there under English.
func TestNotesFollowTheInterfaceLanguage(t *testing.T) {
	reg, fsys, env := scanMachine()
	reg.AddValue(runKey, "Unknown", platform.Value{Data: `%NOPE%\app.exe`, Kind: platform.ValueString})

	restore := i18n.Current()
	t.Cleanup(func() { i18n.Use(restore) })

	i18n.Use(i18n.LangRU)
	_, notes := scanRun(t, reg, fsys, env)
	if !strings.Contains(strings.Join(notes, "\n"), "неизвестная переменная") {
		t.Errorf("notes in Russian = %v", notes)
	}

	i18n.Use(i18n.LangEN)
	_, notes = scanRun(t, reg, fsys, env)
	if !strings.Contains(strings.Join(notes, "\n"), "unknown variable") {
		t.Errorf("notes in English = %v", notes)
	}
}
