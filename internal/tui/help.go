package tui

import (
	"github.com/charmbracelet/bubbletea"

	"github.com/webcartel/sweep/internal/i18n"
)

const (
	screenHelp = iota + 7 // the pop-up of the keys?
)

// The keys of the question open the pop-up of the help from every screen
// but the run; esc and ? close it again.
func (m *Model) updateHelp(msg tea.Msg) (tea.Cmd, bool) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	switch key.String() {
	case "?", "esc", "q", "enter":
		m.screen = m.screenBehind
		return nil, true
	}
	return nil, true
}

// helpHistory remembers the screen the help came out of.
func (m *Model) openHelp() {
	m.screenBehind = m.screen
	m.screen = screenHelp
}

// viewHelp draws what sweep does and every key, in the order a newcomer
// needs them.
func (m *Model) viewHelp() string {
	b := &body{}
	b.add(titleStyle.Render(i18n.T("tui.help.title")))
	for _, line := range wrapText(i18n.T("tui.help.intro"), m.screenWidth()-8) {
		b.add(line)
	}
	b.add("")
	b.add(titleStyle.Render(i18n.T("tui.help.keys")))
	rows := [][2]string{
		{"Up/Down or J K", i18n.T("tui.help.rows.move")},
		{"Space", i18n.T("tui.help.rows.choose")},
		{"A / N", i18n.T("tui.help.rows.allsafe")},
		{"/", i18n.T("tui.help.rows.find")},
		{"O", i18n.T("tui.help.rows.sort")},
		{"Enter or D", i18n.T("tui.help.rows.detail")},
		{"C", i18n.T("tui.help.rows.continue")},
		{"T", i18n.T("tui.help.rows.bin")},
		{"R", i18n.T("tui.help.rows.again")},
		{"S", i18n.T("tui.help.rows.save")},
		{"? ", i18n.T("tui.help.rows.help")},
		{"Q / Esc", i18n.T("tui.help.rows.quit")},
	}
	for _, row := range rows {
		b.add("  " + fit(row[0], 12) + mutedStyle.Render(row[1]))
	}
	b.add("")
	b.add(mutedStyle.Render(i18n.T("tui.help.mouse")))

	return m.renderFrame(i18n.T("tui.help.step"), *b,
		mutedStyle.Render(i18n.T("tui.help.closehint")), makeButtons(
			buttonSpec{label: i18n.T("tui.help.close"), primary: true, action: actCloseHelp},
		))
}
