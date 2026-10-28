package tui

import (
	"github.com/charmbracelet/bubbletea"

	"github.com/webcartel/sweep/internal/i18n"
	"github.com/webcartel/sweep/internal/settings"
)

// screenLang is the picker of the interface language. It follows the rules
// screens, which is where the rest of the settings live.
const screenLang = 20

// langChoice is one language the picker lists. The name stays in its own
// language on purpose: a user who cannot read the screen yet must still
// find the way back to the one they read.
type langChoice struct {
	lang   i18n.Lang
	label  string
	action string
}

// langChoices are the languages the picker offers, in the order shown.
var langChoices = []langChoice{
	{i18n.LangEN, "English", actLangEN},
	{i18n.LangRU, "Русский", actLangRU},
}

// openLanguage puts the picker on screen and remembers the language it
// opened with: cancelling has to put that one back.
func (m *Model) openLanguage() {
	m.langEntry = i18n.Current()
	m.langCursor = 0
	for i, one := range langChoices {
		if one.lang == m.langEntry {
			m.langCursor = i
		}
	}
	m.langNotice = ""
	m.langFailed = false
	m.screen = screenLang
}

// previewLanguage shows a language at once without saving it, so the
// choice is judged by its result rather than by its name. Nothing is
// written until the user confirms.
func (m *Model) previewLanguage(lang i18n.Lang) tea.Cmd {
	i18n.Use(lang)
	m.langNotice = ""
	m.langFailed = false
	return tea.SetWindowTitle(i18n.T("app.title"))
}

// saveLanguage keeps the language the picker shows and returns to the
// settings. A write that fails keeps the user on the picker with the
// reason, so the choice is not dropped in silence.
func (m *Model) saveLanguage() tea.Cmd {
	if m.settingsPath == "" {
		m.langNotice = i18n.T("tui.lang.nosettings")
		m.langFailed = true
		return nil
	}
	if err := settings.Write(m.settingsPath, string(i18n.Current())); err != nil {
		m.langNotice = i18n.T("tui.lang.savefailed") + ": " + err.Error()
		m.langFailed = true
		return nil
	}
	m.langNotice = i18n.T("tui.lang.saved")
	m.langFailed = false
	m.screen = screenRules
	return tea.SetWindowTitle(i18n.T("app.title"))
}

// cancelLanguage puts back the language the picker opened with and
// returns to the settings without saving anything.
func (m *Model) cancelLanguage() tea.Cmd {
	i18n.Use(m.langEntry)
	m.langNotice = ""
	m.langFailed = false
	m.screen = screenRules
	return tea.SetWindowTitle(i18n.T("app.title"))
}

// viewLang draws the languages as rows of the page and keeps the footer
// for the two answers, confirm and cancel.
func (m *Model) viewLang() string {
	b := &body{}
	b.add(titleStyle.Render(i18n.T("tui.lang.title")))
	b.add(mutedStyle.Render(i18n.T("tui.lang.hint")))
	b.add("")

	for i, one := range langChoices {
		pointer := "  "
		if i == m.langCursor {
			pointer = m.skin.pointer() + " "
		}
		mark := m.skin.checkbox(one.lang == i18n.Current())
		line := pointer + mark + " " + one.label
		if i == m.langCursor {
			line = selectedStyle.Render(line)
		}
		index := b.add(line)
		b.hit(index, 0, m.screenWidth()-8, one.action, "")
	}

	if m.langNotice != "" {
		b.add("")
		if m.langFailed {
			b.add(warningStyle().Render(m.langNotice))
		} else {
			b.add(successStyle.Render(m.langNotice))
		}
	}

	hint := m.skin.arrows() + i18n.T("tui.lang.hint.move") + m.skin.sep() +
		i18n.T("tui.lang.hint.save") + m.skin.sep() +
		i18n.T("tui.lang.hint.cancel")
	return m.renderFrame(i18n.T("tui.lang.title"), *b, mutedStyle.Render(hint), makeButtons(
		buttonSpec{label: i18n.T("tui.lang.confirm"), primary: true, action: actLangSave},
		buttonSpec{label: i18n.T("tui.lang.cancel"), action: actLangCancel},
	))
}

// updateLang takes the keys of the picker.
func (m *Model) updateLang(msg tea.Msg) (tea.Cmd, bool) {
	if wheel, ok := msg.(tea.MouseMsg); ok {
		switch wheel.Button {
		case tea.MouseButtonWheelUp:
			msg = tea.KeyMsg{Type: tea.KeyUp}
		case tea.MouseButtonWheelDown:
			msg = tea.KeyMsg{Type: tea.KeyDown}
		}
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	switch key.String() {
	case "esc", "q":
		return m.cancelLanguage(), true
	case "up", "k", "left", "h":
		if m.langCursor > 0 {
			m.langCursor--
			return m.previewLanguage(langChoices[m.langCursor].lang), true
		}
	case "down", "j", "right", "l":
		if m.langCursor < len(langChoices)-1 {
			m.langCursor++
			return m.previewLanguage(langChoices[m.langCursor].lang), true
		}
	case " ":
		return m.previewLanguage(langChoices[m.langCursor].lang), true
	case "enter":
		return m.saveLanguage(), true
	case "e":
		m.langCursor = 0
		return m.previewLanguage(i18n.LangEN), true
	case "r":
		m.langCursor = 1
		return m.previewLanguage(i18n.LangRU), true
	}
	return nil, true
}
