package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbletea"

	"github.com/webcartel/sweep/internal/platform"
	"github.com/webcartel/sweep/internal/platform/fake"
)

// registryModel is the screens over a registry and a disk in memory.
func registryModel(t *testing.T) (*Model, *fake.Registry, string) {
	t.Helper()
	reg := fake.NewRegistry()
	fsys := fake.NewFS()
	fsys.SetDrive(`C:\`, platform.DriveFixed)
	env := fake.NewEnv()
	local := t.TempDir()
	env.Set("LOCALAPPDATA", local)
	run := `HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Run`
	reg.AddValue(run, "Gone", platform.Value{Data: `C:\Gone\app.exe`, Kind: platform.ValueString})
	reg.AddValue(run, "Live", platform.Value{Data: `C:\Live\app.exe`, Kind: platform.ValueString})
	fsys.Add(platform.Info{Path: `C:\Live\app.exe`})
	m := New(Deps{Profiles: testProfiles, AsciiFlag: true, NoASCIIFlag: false,
		Registry: reg, FS: fsys, Env: env, System: fake.NewSystem()})
	return m, reg, local
}

// drive runs the commands a screen returns until there are none.
func drive(m *Model, cmd tea.Cmd) *Model {
	for cmd != nil {
		msg := cmd()
		next, nextCmd := m.Update(msg)
		m = next.(*Model)
		cmd = nextCmd
	}
	return m
}

// The registry flow reads, lists, and starts with nothing ticked.
func TestRegistryFlowOpens(t *testing.T) {
	m, _, _ := registryModel(t)
	m = drive(m, m.openRegistry())
	if m.screen != screenRegReview {
		t.Fatalf("screen = %d, want the review", m.screen)
	}
	if len(m.findings) != 1 {
		t.Fatalf("findings = %+v, want the dead value", m.findings)
	}
	if m.selectedCount() != 0 {
		t.Fatalf("the flow starts with %d ticked", m.selectedCount())
	}
	if !strings.Contains(m.View(), "Gone") {
		t.Fatalf("the review hides the finding: %q", m.View())
	}
}

// Space ticks a finding and the confirm screen runs the cleaner.
func TestRegistryFlowCleans(t *testing.T) {
	m, reg, local := registryModel(t)
	m = drive(m, m.openRegistry())
	m = keyIn(m, "", tea.KeySpace)
	if m.selectedCount() != 1 {
		t.Fatalf("space ticked %d", m.selectedCount())
	}
	m = keyIn(m, "c", 0)
	if m.screen != screenRegConfirm {
		t.Fatalf("c left the screen at %d", m.screen)
	}
	m = drive(m, m.startRegistryClean())
	if m.screen != screenRegDone {
		t.Fatalf("screen = %d, want done: %s", m.screen, m.errText)
	}
	if m.regResult.Removed != 1 {
		t.Fatalf("result = %+v", m.regResult)
	}
	if !strings.HasPrefix(m.regResult.BackupDir, local) {
		t.Fatalf("backup = %q, want it under %q", m.regResult.BackupDir, local)
	}
	if len(reg.Operations) == 0 || !strings.HasPrefix(reg.Operations[0], "export") {
		t.Fatalf("operations = %v", reg.Operations)
	}
}

// A click on a row ticks it.
func TestRegistryMouseTicks(t *testing.T) {
	m, _, _ := registryModel(t)
	m = drive(m, m.openRegistry())
	m = clickAction(t, m, actRegToggle)
	if m.selectedCount() != 1 {
		t.Fatalf("the click ticked %d", m.selectedCount())
	}
}

// A restart brings the choice back through the preselect list.
func TestRegistryPreselectSurvivesRestart(t *testing.T) {
	m, _, _ := registryModel(t)
	m = drive(m, m.openRegistry())
	id := m.findings[0].ID

	fresh, _, _ := registryModel(t)
	fresh.registryPreselect = []string{id}
	fresh = drive(fresh, fresh.openRegistry())
	if fresh.selectedCount() != 1 {
		t.Fatalf("the preselect restored %d findings", fresh.selectedCount())
	}
}
