package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/report"
)

const (
	screenRunning = iota + 5 // the run at work
)

// confirmState is what the modality of the run remembers: the letters the
// user typed, for the word the dangerous rules ask for.
type confirmState struct {
	confirmText string
}

// startRun takes the run the confirm modal agrees to; the command it hands
// over makes the executor work. The executor asks nothing once the modal
// said its word.
func (m *Model) startRun() tea.Cmd {
	// The executor comes with task 8; the screen is in place now.
	return nil
}

// selectedDangerous says whether the choice holds a rule the run asks a
// word for.
func (m *Model) selectedDangerous() bool {
	for _, rp := range m.plan.Rules {
		if rp.Rule == nil {
			continue
		}
		if rp.Rule.Risk == model.RiskDangerous && m.selection.Rules[rp.Rule.ID] {
			return true
		}
	}
	return false
}

// updateConfirm, the part of the modality of the confirmation.
func (m *Model) updateConfirm(msg tea.Msg) (tea.Cmd, bool) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	switch key.String() {
	case "esc":
		m.screen = screenList
		m.confirmText = ""
		return nil, true
	case "enter":
		if m.selectedDangerous() && m.confirmText != "yes" {
			return nil, true
		}
		m.screen = screenRunning
		return m.startRun(), true
	}
	runes := key.Runes
	if len(runes) != 0 && runes[0] >= 'a' && runes[0] <= 'z' {
		m.confirmText += string(runes)
	}
	return nil, true
}

// viewConfirm draws what the run takes, the dangerous rules it asks a word
// for, and the keys of the question.
func (m *Model) viewConfirm() string {
	rules, _, bytes := m.totals()
	var lines []string
	lines = append(lines, fmt.Sprintf("clean %d rules, %s", rules, report.FormatBytes(bytes)))
	trashWord := "deleted for good"
	if m.trash {
		trashWord = "sent to the bin"
	}
	lines = append(lines, trashWord)
	var danger []string
	for _, rp := range m.plan.Rules {
		if rp.Rule != nil && rp.Rule.Risk == model.RiskDangerous && m.selection.Rules[rp.Rule.ID] {
			danger = append(danger, rp.Rule.ID)
		}
	}
	if len(danger) != 0 {
		lines = append(lines, "dangerous: "+strings.Join(danger, ", ")+" - type yes to run")
		lines = append(lines, "> "+m.confirmText)
	}
	lines = append(lines, "enter go  esc back")
	return strings.Join(lines, "\n")
}
