package tui

import (
	"testing"

	"github.com/charmbracelet/bubbletea"
	"github.com/webcartel/sweep/internal/model"
)

// chomped drives a scan of two rules to its plan and leaves the model on
// the screen of the list.
func chomped(t *testing.T) *Model {
	t.Helper()
	scanner := testMachine(t)
	rules := twoScanRules(`%LOCALAPPDATA%\big`)
	m := New(Deps{Profiles: testProfiles, Scanner: scanner,
		RulesFor: func(string) ([]*model.Rule, error) { return rules, nil }})
	cur, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	for cmd != nil {
		msg := cmd()
		cur, cmd = cur.Update(msg)
	}
	return cur.(*Model)
}

// stepIn feeds one key into the model: an arrow key, or a letter.
func stepIn(m *Model, key tea.KeyType, letter ...byte) *Model {
	var msg tea.Msg
	if len(letter) != 0 {
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{rune(letter[0])}}
	} else {
		msg = tea.KeyMsg{Type: key}
	}
	next, _ := m.Update(msg)
	return next.(*Model)
}

// The screen of the list shows the totals of what the choice keeps.
func TestListTotals(t *testing.T) {
	m := chomped(t)
	if m.screen != screenList {
		t.Fatalf("screen = %d, want screenList", m.screen)
	}
	// Both rules count the one file; the default choice keeps the safe one.
	rules, files, bytes := m.totals()
	if rules != 1 || files != 1 || bytes != 1 {
		t.Fatalf("totals of the default choice: rules=%d files=%d bytes=%d, want 1,1,1", rules, files, bytes)
	}
}

// The keys of the choice: all safe in with a, every rule out with n, and a
// rule in and out with the space of its row.
func TestListToggleSelection(t *testing.T) {
	m := chomped(t)
	m = stepIn(m, 0, 'n')
	if rules, _, _ := m.totals(); rules != 0 {
		t.Fatalf("after n the choice holds %d rules, want 0", rules)
	}
	m = stepIn(m, 0, 'a')
	if rules, _, _ := m.totals(); rules != 1 {
		t.Fatalf("after a the choice holds %d rules, want 1", rules)
	}
	m = stepIn(m, 0, ' ')
	if rules, _, _ := m.totals(); rules != 0 {
		t.Fatalf("after space the choice holds %d rules, want 0", rules)
	}
	m = stepIn(m, 0, ' ')
	if rules, _, _ := m.totals(); rules != 1 {
		t.Fatalf("after space twice the choice holds %d rules, want 1", rules)
	}
}

// The filter keeps rows whose id or name holds the letters typed.
func TestListFilter(t *testing.T) {
	m := chomped(t)
	if got := len(m.visibleRows()); got != 2 {
		t.Fatalf("the unfiltered list shows %d rows, want 2", got)
	}
	m = stepIn(m, 0, '/')
	if !m.filtering {
		t.Fatalf("the slash does not open the filter")
	}
	m = stepIn(m, 0, 's')
	m = stepIn(m, 0, 'a')
	m = stepIn(m, 0, 'f')
	if m.filter != "saf" {
		t.Fatalf("the filter holds %q, want saf", m.filter)
	}
	rows := m.visibleRows()
	if len(rows) != 1 || rows[0].Rule.ID != "safe.rule" {
		t.Fatalf("the filter left %d rows, want the one of safe.rule", len(rows))
	}
	m = stepIn(m, tea.KeyEsc)
	if m.filtering || m.filter != "" {
		t.Fatalf("esc left the filter on: %q", m.filter)
	}
	if got := len(m.visibleRows()); got != 2 {
		t.Fatalf("after closing the filter the list shows %d rows, want 2", got)
	}
}

// The sort by size trades the one by name on o.
func TestListSort(t *testing.T) {
	m := chomped(t)
	if m.sort != "size" {
		t.Fatalf("the list starts sorted by %q, want size", m.sort)
	}
	m = stepIn(m, 0, 'o')
	if m.sort != "name" {
		t.Fatalf("after o the list is sorted by %q, want name", m.sort)
	}
	rows := m.visibleRows()
	if len(rows) != 2 || rows[0].Rule.ID != "danger.rule" {
		t.Fatalf("the name sort came with %d rows, first %v", len(rows), rows[0].Rule.ID)
	}
}

// The trash is the whole run's say: it starts off, and one t turns it over.
func TestListTrashToggle(t *testing.T) {
	m := chomped(t)
	if m.trash {
		t.Fatalf("the run starts deleting for good, want the bin")
	}
	m = stepIn(m, 0, 't')
	if !m.trash {
		t.Fatalf("t did not turn the bin on")
	}
}
