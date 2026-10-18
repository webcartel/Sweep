package tui

import (
	"testing"

	"github.com/charmbracelet/bubbletea"
)

// Enter on a row opens the detail of its rule; the detail lists what is
// under the rule, and a space rules one item out of the run.
func TestDetailOpensAndExcludes(t *testing.T) {
	m := chomped(t)
	m = stepIn(m, tea.KeyEnter) // the row of the safe rule
	if m.screen != screenDetail {
		t.Fatalf("screen = %d, want screenDetail (%d)", m.screen, screenDetail)
	}
	items := m.detailItems()
	if len(items) != 1 {
		t.Fatalf("the detail shows %d items, want 1", len(items))
	}
	if items[0].Path != `C:\Users\probe\AppData\Local\big\f0` {
		t.Fatalf("the item is %q", items[0].Path)
	}
	m = stepIn(m, 0, ' ') // the item out from the run
	if !m.selection.Exclude[items[0].Path] {
		t.Fatalf("the space does not exclude the item")
	}
	m = stepIn(m, tea.KeyEsc)
	if m.screen != screenList {
		t.Fatalf("esc does not lead back to the list, screen = %d", m.screen)
	}
	if _, files, _ := m.totals(); files != 0 {
		t.Fatalf("the excluded item still counts %d files", files)
	}
}

// Space once more takes the item back in.
func TestDetailIncludesBack(t *testing.T) {
	m := chomped(t)
	m = stepIn(m, tea.KeyEnter)
	path := m.detailItems()[0].Path
	m = stepIn(m, 0, ' ')
	m = stepIn(m, 0, ' ')
	if m.selection.Exclude[path] {
		t.Fatalf("the second space does not take the item back")
	}
}
