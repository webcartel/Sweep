package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbletea"
)

// chosenDangerous turns the double rules over, so the dangerous rule is in
// the run and the safe one out.
func chosenDangerous(m *Model) *Model {
	m.selection.Rules["safe.rule"] = false
	m.selection.Rules["danger.rule"] = true
	return m
}

// The list opens the modality of the confirmation on d; the modal says what
// the run takes, and from there back to the list stands on esc.
func TestConfirmModal(t *testing.T) {
	m := chomped(t)
	m = stepIn(m, 0, 'c')
	if m.screen != screenConfirm {
		t.Fatalf("screen = %d, want screenConfirm (%d)", m.screen, screenConfirm)
	}
	view := m.View()
	if !strings.Contains(view, "1 rule") {
		t.Fatalf("the modal does not count the rules in %q", view)
	}
	m = stepIn(m, tea.KeyEsc)
	if m.screen != screenList {
		t.Fatalf("esc does not leave the modal, screen = %d", m.screen)
	}
}

// A run without dangerous rules starts on Enter.
func TestConfirmStartsRun(t *testing.T) {
	m := chomped(t)
	m = stepIn(m, 0, 'c')
	cur, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = cur.(*Model)
	if m.screen != screenRunning {
		t.Fatalf("screen = %d, want screenRunning (%d)", m.screen, screenRunning)
	}
}

// A run with dangerous rules does not start until the word yes is on the
// screen.
func TestConfirmDangerousNeedsYes(t *testing.T) {
	m := chosenDangerous(chomped(t))
	m = stepIn(m, 0, 'c')
	m = stepIn(m, tea.KeyEnter) // no word yet
	if m.screen != screenConfirm {
		t.Fatalf("the run started without the word, screen = %d", m.screen)
	}
	for _, letter := range []byte{'y', 'e', 's'} {
		m = stepIn(m, 0, letter)
	}
	if m.confirmText != "yes" {
		t.Fatalf("the screen holds %q", m.confirmText)
	}
	m = stepIn(m, tea.KeyEnter)
	if m.screen != screenRunning {
		t.Fatalf("the word did not start the run, screen = %d", m.screen)
	}
}


