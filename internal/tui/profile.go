package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/report"
)

// Additional screens of the run.
const (
	screenScanning = iota + 1
)

// ProfileRow is one line of the screen of the profile: what a profile of
// the set is named and what watching it promises.
type ProfileRow struct {
	Name    string
	Rules   int
	MaxRisk model.Risk
}

// Model additions: the profiles of the machine and where the cursor of the
// profile screen stands.
type profileState struct {
	profiles []ProfileRow
	cursor   int
	// pickedProfile is the profile the scan answers with; the screens of
	// the list and of the run name it in the header.
	pickedProfile string
}

// Update, the part of the profile screen: arrows move the cursor, Enter
// opens the scan, q leaves the run.
func (m *Model) updateProfile(msg tea.Msg) (tea.Cmd, bool) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	switch key.String() {
	case "q", "esc", "ctrl+c":
		return tea.Quit, true
	case "?":
		m.openHelp()
		return nil, true
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

// viewScanning draws the scan at work: the rules in the walk, the notes
// the scanner had, and the error that broke the run, if one did.
func (m *Model) viewScanning() string {
	lines := []string{"scanning " + m.profileOfHeader()}
	for id, plan := range m.progress {
		verb := fmt.Sprintf("%d files, %s", plan.Files, report.FormatBytes(plan.Bytes))
		lines = append(lines, fmt.Sprintf("%-14s %s", id, verb))
	}
	lines = append(lines, m.notes...)
	if m.scanError != nil {
		lines = append(lines, "the scan failed: "+m.scanError.Error())
	}
	lines = append(lines, "esc stand down")
	out := ""
	for _, line := range lines {
		out += line + "\n"
	}
	return out
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
