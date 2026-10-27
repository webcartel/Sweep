package tui

import (
	"fmt"

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
	b.add(titleStyle.Render(fmt.Sprintf("Ready to clean %s", report.FormatBytes(bytes))))
	_, about := humanProfile(m.profileOfHeader())
	b.add(mutedStyle.Render(fmt.Sprintf("%s%s%s files%s%s",
		ruleCount(rules), m.skin.sep(), humanCount(files), m.skin.sep(), about)))
	b.add("")

	if m.trash {
		b.add("Files go to the Recycle Bin. You can restore them if you change your mind.")
	} else {
		b.add("Files are deleted permanently. They cannot be restored.")
	}
	b.add(mutedStyle.Render("Safety check runs again before every deletion."))

	danger := m.selectedDangerous()
	if danger {
		b.add("")
		b.add(dangerStyle.Render("Some chosen rules may remove data you still want:"))
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
			b.add(safeStyle.Render("Type yes to allow these rules: ") + "yes")
		} else {
			b.add("Type yes to allow these rules: " + m.confirmText + m.skin.caret())
		}
	}

	if m.adminNeeded() {
		b.add("")
		b.add(warningStyle().Render("Some chosen rules need administrator rights."))
		b.add(mutedStyle.Render("Restart sweep as administrator to include them."))
	}

	buttons := []buttonSpec{{label: "Yes, clean now", primary: true, action: actClean}}
	if m.adminNeeded() {
		buttons = append(buttons, buttonSpec{label: "Restart as administrator", action: actRestart})
	}
	buttons = append(buttons, buttonSpec{label: "Go back", action: actBack})

	hint := "Enter clean" + m.skin.sep() + "Esc back" + m.skin.sep() + "T recycle bin"
	return m.renderFrame("Step 4 of 4", *b, mutedStyle.Render(hint), makeButtons(buttons...))
}

// pendingWord answers whether the risky rules still wait for their word.
func (m *Model) pendingWord() bool {
	return m.selectedDangerous() && m.confirmText != "yes"
}

// riskySentence is the warning of the confirm screen in one line, kept for
// the screens that name it again.
