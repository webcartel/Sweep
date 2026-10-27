package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbletea"
)

// doneFromRun drives the run of the machine to its end, with a log
// directory of its own.
func doneFromRun(t *testing.T) *Model {
	t.Helper()
	dir := t.TempDir()
	m := doneMachine(t, dir)
	_ = m.logDir
	cur := tea.Model(m)
	cmd := m.run.execCmd
	for cmd != nil {
		msg := cmd()
		cur, cmd = cur.Update(msg)
	}
	return cur.(*Model)
}

// The done screen says what the run came to, with the bytes it freed.
func TestDoneSummary(t *testing.T) {
	m := doneFromRun(t)
	if m.screen != screenDone {
		t.Fatalf("screen = %d, want screenDone", m.screen)
	}
	view := m.View()
	if !strings.Contains(view, "1 B") {
		t.Fatalf("the done screen hides the bytes freed in %q", view)
	}
}

// The s of the done screen saves the answer of the run next to the log.
func TestDoneSavesReport(t *testing.T) {
	m := doneFromRun(t)
	logDir := m.logDir
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	found := false
	answers, err := os.ReadDir(filepath.Join(logDir, "sweep", "logs"))
	if err != nil {
		t.Fatalf("the log directory went away: %v", err)
	}
	for _, answer := range answers {
		if strings.HasPrefix(answer.Name(), "sweep-report-") && strings.HasSuffix(answer.Name(), ".json") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the answer of the run is not next to the log in %q", logDir)
	}
}

// The save button of the result screen writes the report and says where.
func TestDoneSaveButtonSaysWhere(t *testing.T) {
	m := doneFromRun(t)
	m = clickAction(t, m, actSaveReport)
	if !strings.Contains(m.run.saveNote, "Report saved:") {
		t.Fatalf("the save button said %q", m.run.saveNote)
	}
	if !strings.Contains(m.View(), "Report saved:") {
		t.Fatalf("the result screen hides the note: %q", m.View())
	}
}

// q leaves the run on the way out.
func TestDoneQuits(t *testing.T) {
	m := doneFromRun(t)
	_, quit := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if quit == nil {
		t.Fatalf("q does not ask bubble tea to quit")
	}
}
