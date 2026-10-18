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
type Deps struct{}

// Model is the one Bubble Tea model of the TUI. Every screen of the run is a
// state of it.
type Model struct {
	screen int
}

// New puts the model together. The TUI opens on the screen of the profile.
func New(_ Deps) *Model {
	return &Model{screen: screenProfile}
}

// Init starts the first command of the program; none yet.
func (m *Model) Init() tea.Cmd { return nil }

// Update handles one message and answers with the model and the next
// command; the screens fill it in as the plan lands them.
func (m *Model) Update(_ tea.Msg) (tea.Model, tea.Cmd) { return m, nil }

// View draws the screen the model is on.
func (m *Model) View() string { return "" }
