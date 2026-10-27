package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/webcartel/sweep/internal/model"
)

// The frame fills the terminal exactly: no line is wider than the window, so
// the border never wraps, and every screen draws inside it.
func TestFrameFitsTheTerminal(t *testing.T) {
	for _, width := range []int{80, 100, 140} {
		m := aPaperModel(true)
		next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
		m = next.(*Model)
		for _, screen := range []int{screenProfile, screenList, screenConfirm, screenDetail, screenDone, screenHelp} {
			m.screen = screen
			if screen == screenDetail {
				m.detailRule = "safe.rule"
			}
			for _, line := range strings.Split(m.View(), "\n") {
				if got := lipgloss.Width(line); got != width-1 {
					t.Fatalf("width %d, screen %d: a line is %d wide: %q", width, screen, got, line)
				}
			}
		}
	}
}

// Rules that found nothing stay out of the list until the user asks for
// them.
func TestListHidesEmptyRules(t *testing.T) {
	m := aPaperModel(true)
	m.plan.Rules = append(m.plan.Rules, model.RulePlan{
		Rule: &model.Rule{ID: "empty.rule", Name: "Empty rule", Risk: model.RiskSafe},
	})
	if got := len(m.visibleRows()); got != 2 {
		t.Fatalf("the list shows %d rows, want the two that found files", got)
	}
	m = stepIn(m, 0, 'e')
	if got := len(m.visibleRows()); got != 3 {
		t.Fatalf("after e the list shows %d rows, want all three", got)
	}
}

// The start screen names the three ways to clean in plain words, not in the
// names of the profiles alone.
func TestProfileSpeaksPlainly(t *testing.T) {
	m := New(Deps{Profiles: testProfiles, AsciiFlag: true, NoASCIIFlag: false})
	view := m.View()
	for _, want := range []string{"Free space safely", "Everyday cleanup", "Deep cleanup", "Start"} {
		if !strings.Contains(view, want) {
			t.Fatalf("the start screen does not say %q in %q", want, view)
		}
	}
}

// A window smaller than the spec's 80 x 24 asks to be enlarged instead of
// squeezing the screens.
func TestTinyWindowAsksToEnlarge(t *testing.T) {
	m := aPaperModel(true)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	m = next.(*Model)
	if !strings.Contains(m.View(), "make the window larger") {
		t.Fatalf("a 60x18 window is not refused: %q", m.View())
	}
}

// The done screen leads with the space it freed.
func TestDoneLeadsWithFreedSpace(t *testing.T) {
	m := doneFromRun(t)
	view := m.View()
	first := strings.SplitN(view, "\n", 4)[2]
	if !strings.Contains(first, "Freed") {
		t.Fatalf("the first line of the result is %q, want the freed space", first)
	}
}
