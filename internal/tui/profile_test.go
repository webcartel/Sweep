package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbletea"
	"github.com/webcartel/sweep/internal/model"
)

var testProfiles = []ProfileRow{
	{Name: "default", Rules: 5, MaxRisk: model.RiskSafe},
	{Name: "aggressive", Rules: 12, MaxRisk: model.RiskDangerous},
}

// The screen of the profile moves the cursor with the arrows and opens the
// scan on Enter.
func TestProfileScreenNavigation(t *testing.T) {
	m := New(Deps{Profiles: testProfiles})
	if m.cursor != 0 {
		t.Fatalf("cursor starts at %d, want 0", m.cursor)
	}
	step := func(key tea.KeyType) *Model {
		next, _ := m.Update(tea.KeyMsg{Type: key})
		return next.(*Model)
	}
	m = step(tea.KeyDown)
	if m.cursor != 1 {
		t.Fatalf("after down cursor = %d, want 1", m.cursor)
	}
	m = step(tea.KeyUp)
	if m.cursor != 0 {
		t.Fatalf("after up cursor = %d, want 0", m.cursor)
	}
	m = step(tea.KeyUp)
	if m.cursor != 0 {
		t.Fatalf("up above the first line fell to %d, want 0", m.cursor)
	}
	m = step(tea.KeyEnter)
	if m.screen != screenScanning {
		t.Fatalf("after enter screen = %d, want screenScanning (%d)", m.screen, screenScanning)
	}
}

// Every profile on this machine is named once on the screen.
func TestProfileViewNamesProfiles(t *testing.T) {
	m := New(Deps{Profiles: testProfiles})
	view := m.View()
	for _, profile := range testProfiles {
		if got := strings.Count(view, profile.Name); got != 1 {
			t.Fatalf("profile %s named %d times in %q", profile.Name, got, view)
		}
	}
}
