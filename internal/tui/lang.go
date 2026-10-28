package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbletea"

	"github.com/webcartel/sweep/internal/i18n"
	"github.com/webcartel/sweep/internal/settings"
)

// screenLang is the picker of the interface language. It follows the rules
// screens, which is where the rest of the settings live.
const screenLang = 20

// setLanguage puts a language in charge and saves it for the next run. It
// answers no command: the screens redraw in the new language on the very
// next frame, because every one of them asks i18n for its text.
func (m *Model) setLanguage(lang i18n.Lang) tea.Cmd {
	i18n.Use(lang)
	m.langFailed = false

	if m.settingsPath == "" {
		m.langNotice = i18n.T("tui.lang.nosettings")
		m.langFailed = true
		return nil
	}
	if err := settings.Write(m.settingsPath, string(lang)); err != nil {
		m.langNotice = i18n.T("tui.lang.savefailed") + ": " + err.Error()
		m.langFailed = true
		return nil
	}
	m.langNotice = i18n.T("tui.lang.saved")
	return nil
}

// viewLang draws the picker. The two names stay in their own language on
// purpose: a user who cannot read the screen yet must still be able to
// find the way back to the one they read.
func (m *Model) viewLang() string {
	b := &body{}
	b.add(titleStyle.Render(i18n.T("tui.lang.title")))
	b.add(mutedStyle.Render(i18n.T("tui.lang.hint")))
	b.add("")
	b.add(fmt.Sprintf(i18n.T("tui.lang.current"), string(i18n.Current())))
	if m.langNotice != "" {
		b.add("")
		if m.langFailed {
			b.add(warningStyle().Render(m.langNotice))
		} else {
			b.add(successStyle.Render(m.langNotice))
		}
	}
	b.add("")
	if m.langNotice != "" {
		if m.langFailed {
			b.add(warningStyle().Render(m.langNotice))
		} else {
			b.add(successStyle.Render(m.langNotice))
		}
		b.add("")
	}

	hint := "E English" + m.skin.sep() + "R Русский" + m.skin.sep() + "Esc back"
	return m.renderFrame(i18n.T("tui.lang.title"), *b, mutedStyle.Render(hint), makeButtons(
		buttonSpec{label: "English", primary: i18n.Current() == i18n.LangEN, action: actLangEN},
		buttonSpec{label: "Русский", primary: i18n.Current() == i18n.LangRU, action: actLangRU},
		buttonSpec{label: i18n.T("tui.button.back"), action: actBack},
	))
}

// updateLang takes the keys of the picker.
func (m *Model) updateLang(msg tea.Msg) (tea.Cmd, bool) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	switch key.String() {
	case "esc", "q":
		m.screen = screenRules
	case "e":
		return m.setLanguage(i18n.LangEN), true
	case "r":
		return m.setLanguage(i18n.LangRU), true
	}
	return nil, true
}
