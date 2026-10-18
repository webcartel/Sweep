// Package tui is the terminal frontend of sweep: it scans, shows and cleans
// through the same core packages the CLI commands call, and adds nothing to
// the work itself.
package tui

import "github.com/charmbracelet/bubbletea"

// The screens of the TUI, in the order a run walks them. A screen is a state
// of one Model, not a type of its own: the plan of the spec.
const (
	screenProfile = iota // the choice of the profile
)

// Deps collects what the TUI reaches the machine through; the frontend that
// builds it is the root command of the CLI.
type Deps struct {
	// Profiles names every profile of the rules, in the order the TUI
	// lists them.
	Profiles []profileRow
}

// Model is the one Bubble Tea model of the TUI. Every screen of the run is a
// state of it.
type Model struct {
	screen int
	profileState
}

// New puts the model together. The TUI opens on the screen of the profile.
func New(_ Deps) *Model {
	return &Model{screen: screenProfile}
}

// Init starts the first command of the program; none yet.
func (m *Model) Init() tea.Cmd { return nil }

// Update handles one message and answers with the model and the next
// command.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.screen {
	case screenProfile:
		cmd, _ = m.updateProfile(msg)
	case screenScanning:
		// The scan waits for task 4.
	}
	return m, cmd
}

// View draws the screen the model is on.
func (m *Model) View() string {
	switch m.screen {
	case screenProfile:
		return m.viewProfile()
	}
	return ""
}
