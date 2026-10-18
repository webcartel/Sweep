package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/webcartel/sweep/internal/model"
)

// Additional screens of the run.
const (
	screenScanning = iota + 1
)

// profileRow is one line of the screen of the profile: what a profile of
// the set is named and what watching it promises.
type profileRow struct {
	Name    string
	Rules   int
	MaxRisk model.Risk
}

// Model additions: the profiles of the machine and where the cursor of the
// profile screen stands.
type profileState struct {
	profiles []profileRow
	cursor   int
}

// Update, the part of the profile screen: arrows move the cursor, Enter
// opens the scan, q leaves the run.
func (m *Model) updateProfile(msg tea.Msg) (tea.Cmd, bool) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	switch key.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return nil, true
	case "down", "j":
		if m.cursor < len(m.profiles)-1 {
			m.cursor++
		}
		return nil, true
	case "enter":
		m.screen = screenScanning
		return m.startScan(), true
	}
	return nil, false
}

// viewProfile draws the choice of the profile.
func (m *Model) viewProfile() string {
	var lines []string
	lines = append(lines, "Clean how much?")
	for i, profile := range m.profiles {
		mark := "  "
		if i == m.cursor {
			mark = "> "
		}
		lines = append(lines, fmt.Sprintf("%s%s (%d rules)", mark, profile.Name, profile.Rules))
	}
	return strings.Join(lines, "\n")
}
