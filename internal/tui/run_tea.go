package tui

import (
	"github.com/charmbracelet/bubbletea"
)

// Run opens the screens of the TUI the machine was built for, with the
// whole screen and the mouse it speaks to.
func Run(d Deps) error {
	m := New(d)
	program := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := program.Run()
	return err
}
