package tui

import "testing"

// The TUI opens on the screen of the profile, the way the user agreed:
// the first thing it asks is which profile to scan.
func TestNewStartsOnProfileScreen(t *testing.T) {
	m := New(Deps{})
	if m.screen != screenProfile {
		t.Fatalf("initial screen = %d, want screenProfile (%d)", m.screen, screenProfile)
	}
}
