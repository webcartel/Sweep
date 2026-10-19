package tui

import (
	"testing"

	"github.com/charmbracelet/bubbletea"
	"github.com/webcartel/sweep/internal/exec"
	"github.com/webcartel/sweep/internal/guard"
	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/platform/fake"
)

// runMachine builds the whole machine of a run - the fake of the disk, the
// guard, the scanner and the executor that all know it - and drives one
// scan and one modal to the screen of the run.
func runMachine(t *testing.T) (*fake.FS, *Model) {
	t.Helper()
	fsys, _, m := withLogMachine(t, "")
	return fsys, m
}

// doneMachine drives a machine the same way, with a log directory the
// answer of the run can be saved next to.
func doneMachine(t *testing.T, logDir string) *Model {
	_, _, m := withLogMachine(t, logDir)
	return m
}

// withLogMachine builds the whole machine of a run - the fake of the disk,
// the guard, the scanner and the executor that all know it - and drives one
// scan and one modal to the screen of the run.
func withLogMachine(t *testing.T, logDir string) (*fake.FS, *fake.Env, *Model) {
	t.Helper()
	fsys, env, scanner := machineParts(t)
	guarded, err := guard.New(fsys, env, `C:\Users\probe`, nil)
	if err != nil {
		t.Fatalf("the guard failed to build: %v", err)
	}
	executor := exec.New(exec.Deps{FS: fsys, Guard: guarded, System: fake.NewSystem()})
	m := New(Deps{
		Profiles: testProfiles, Scanner: scanner, Executor: executor, LogDir: logDir,
		RulesFor: func(string) ([]*model.Rule, error) { return twoScanRules(`%LOCALAPPDATA%\big`), nil },
	})
	// A scan to the plan: the loop of bubble tea, one message at a time.
	cur, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	for cmd != nil {
		msg := cmd()
		cur, cmd = cur.Update(msg)
	}
	// The d of the modal, the Enter of the run.
	for _, key := range []struct {
		typ  tea.KeyType
		text string
	}{{0, "c"}, {tea.KeyEnter, ""}} {
		var msg tea.Msg
		if key.text != "" {
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key.text)}
		} else {
			msg = tea.KeyMsg{Type: key.typ}
		}
		cur, _ = m.Update(msg)
		m = cur.(*Model)
	}
	if m.screen != screenRunning {
		t.Fatalf("screen = %d, want screenRunning", m.screen)
	}
	return fsys, env, m
}

// The run at work: the events add up, and the done screen answers at the
// end with what was taken.
func TestRunningCollectsResult(t *testing.T) {
	_, m := runMachine(t)
	cur := tea.Model(m)
	cmd := m.run.execCmd
	if cmd == nil {
		t.Fatalf("the run started without a command of the stream")
	}
	for cmd != nil {
		msg := cmd()
		cur, cmd = cur.Update(msg)
	}
	done := cur.(*Model)
	if done.screen != screenDone {
		t.Fatalf("screen = %d, want screenDone (%d)", done.screen, screenDone)
	}
	if done.run.result == nil {
		t.Fatalf("the run ended without the result")
	}
	if done.run.result.FreedBytes != 1 {
		t.Fatalf("the run freed %d bytes, want 1", done.run.result.FreedBytes)
	}
}

// The run cut off - the Esc of the run - takes the stream down and leaves
// the modal on the screen again.
func TestRunningEscBackToConfirm(t *testing.T) {
	_, m := runMachine(t)
	if m.run.cancelExec == nil {
		t.Fatalf("the run went without a context of its own")
	}
	cur, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = cur.(*Model)
	if m.screen != screenConfirm {
		t.Fatalf("esc does not lead to the modal, screen = %d", m.screen)
	}
	if m.run.execEvents != nil {
		t.Fatalf("the stream of the run stayed on")
	}
	if m.run.cancelExec != nil {
		t.Fatalf("the context of the run stays prepared")
	}
	m.run.cancelExec = nil
}

// The run takes what the plan said: the file of the fake comes off the
// disk.
func TestRunningRemovesFromDisk(t *testing.T) {
	fsys, m := runMachine(t)
	cur := tea.Model(m)
	cmd := m.run.execCmd
	for cmd != nil {
		msg := cmd()
		cur, cmd = cur.Update(msg)
	}
	removed := fsys.Removed()
	if len(removed) != 1 || removed[0] != `C:\Users\probe\AppData\Local\big\f0` {
		t.Fatalf("the run removed %v, want the one file", removed)
	}
}
