package tui

import (
	"github.com/charmbracelet/bubbletea"
)

// Run opens the screens of the TUI the machine was built for, with the
// whole screen and the mouse it speaks to. A run whose --preselect named
// the rules the word was given skips the choice of the profile: the scan
// goes on its first screen.
func Run(d Deps) error {
	m := New(d)
	program := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := program.Run()
	return err
}
