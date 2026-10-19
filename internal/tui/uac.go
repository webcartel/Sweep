package tui

import (
	"github.com/charmbracelet/bubbletea"
	"github.com/webcartel/sweep/internal/platform"
)

// uacState is what the run remembers about the elevation: whether the
// process has it, and who does the restart.
type uacState struct {
	system    platform.System
	preselect []string
	restart   func(preselect []string) error
}

// adminNeeded says whether the choice holds a rule the process does not
// have the elevation for.
func (m *Model) adminNeeded() bool {
	needed := m.adminRuleIDs()
	return needed.size() != 0
}

// adminRuleIDs answers the rules of the choice that want the elevated
// process, and whether the process is one.
type idSet map[string]bool

func (s idSet) size() int {
	count := 0
	for id, on := range s {
		if on && id != "" {
			count++
		}
	}
	return count
}

func (m *Model) adminRuleIDs() idSet {
	set := idSet{}
	for _, rp := range m.plan.Rules {
		if rp.Rule == nil || !m.selection.Rules[rp.Rule.ID] {
			continue
		}
		if rp.Rule.RequiresAdmin && !m.elevated() {
			set[rp.Rule.ID] = true
		}
	}
	return set
}

// elevated answers whether the process runs elevated.
func (m *Model) elevated() bool {
	if m.system == nil {
		return true
	}
	strong, err := m.system.Elevated()
	if err != nil {
		return true // the unknown asks nothing of the user
	}
	return strong
}

// updateConfirm takes the elevation into the word: where a rule wants the
// elevation, Enter stands for the offer to restart the run as the
// administrator.
func (m *Model) confirmEnter() tea.Cmd {
	needed := m.adminRuleIDs()
	if needed.size() == 0 {
		m.screen = screenRunning
		return m.startRun()
	}
	var preselect []string
	for id := range needed {
		preselect = append(preselect, id)
	}
	if m.restart == nil {
		return nil
	}
	if err := m.restart(preselect); err != nil {
		// The elevation is the user's say: what they turned down leaves
		// the rules that asked for it out of the choice, and the word
		// stands open again.
		for id := range needed {
			m.selection.Rules[id] = false
		}
		return nil
	}
	// The restart left for the next process; this one can stand still.
	return tea.Quit
}

// errNoRestart moved to the frontend: the say of who can restart the run
// is the say of the shell.
