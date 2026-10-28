package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbletea"
	"github.com/webcartel/sweep/internal/i18n"
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
	case "t":
		m.trash = !m.trash
	case "?":
		m.openHelp()
		return nil, true
	case "enter":
		if m.selectedDangerous() && m.confirmText != "yes" {
			return nil, true
		}
		return m.confirmEnter(), true
	}
	runes := key.Runes
	if len(runes) != 0 && runes[0] >= 'a' && runes[0] <= 'z' {
		m.confirmText += string(runes)
	}
	return nil, true
}

// viewConfirm draws the last question before the work: what goes, where it
// goes, and the one word the risky rules ask for.
func (m *Model) viewConfirm() string {
	rules, files, bytes := m.totals()
	b := &body{}
	b.add(titleStyle.Render(fmt.Sprintf(i18n.T("tui.confirm.ready"), report.FormatBytes(bytes))))
	_, about := humanProfile(m.profileOfHeader())
	b.add(mutedStyle.Render(fmt.Sprintf(i18n.T("tui.confirm.summary"),
		ruleCount(rules), m.skin.sep(), humanCount(files), m.skin.sep(), about)))
	b.add("")

	if m.trash {
		b.add(i18n.T("tui.confirm.trash"))
	} else {
		b.add(i18n.T("tui.confirm.permanent"))
	}
	b.add(mutedStyle.Render(i18n.T("tui.confirm.safety")))

	danger := m.selectedDangerous()
	if danger {
		b.add("")
		b.add(dangerStyle.Render(i18n.T("tui.confirm.danger")))
		for _, rp := range m.plan.Rules {
			if rp.Rule != nil && rp.Rule.Risk == model.RiskDangerous && m.selection.Rules[rp.Rule.ID] {
				name := rp.Rule.Name
				if name == "" {
					name = rp.Rule.ID
				}
				b.add("  " + m.skin.bullet() + " " + name)
			}
		}
		b.add("")
		if m.confirmText == "yes" {
			b.add(safeStyle.Render(i18n.T("tui.confirm.typeyes")) + "yes")
		} else {
			b.add(i18n.T("tui.confirm.typeyes") + m.confirmText + m.skin.caret())
		}
	}

	if m.adminNeeded() {
		b.add("")
		b.add(warningStyle().Render(i18n.T("tui.confirm.adminsome")))
		b.add(mutedStyle.Render(i18n.T("tui.confirm.adminrestart")))
	}

	buttons := []buttonSpec{{label: i18n.T("tui.confirm.clean"), primary: true, action: actClean}}
	if m.adminNeeded() {
		buttons = append(buttons, buttonSpec{label: i18n.T("tui.confirm.restart"), action: actRestart})
	}
	buttons = append(buttons, buttonSpec{label: i18n.T("tui.confirm.goback"), action: actBack})

	hint := i18n.T("tui.confirm.hint.clean") + m.skin.sep() +
		i18n.T("tui.hint.back") + m.skin.sep() + i18n.T("tui.confirm.hint.bin")
	return m.renderFrame(i18n.T("tui.frame.step", 4, 4), *b, mutedStyle.Render(hint), makeButtons(buttons...))
}

// pendingWord answers whether the risky rules still wait for their word.
func (m *Model) pendingWord() bool {
	return m.selectedDangerous() && m.confirmText != "yes"
}

// riskySentence is the warning of the confirm screen in one line, kept for
// the screens that name it again.
