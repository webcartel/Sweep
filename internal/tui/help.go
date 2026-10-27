package tui

import (
	"github.com/charmbracelet/bubbletea"
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
	b.add(titleStyle.Render("How sweep works"))
	for _, line := range wrapText("sweep finds files that are safe to remove, shows you the list, and deletes only what you tick. It never stops your apps and never touches Windows itself.", m.screenWidth()-8) {
		b.add(line)
	}
	b.add("")
	b.add(titleStyle.Render("Keys"))
	rows := [][2]string{
		{"Up/Down or J K", "move between rows"},
		{"Space", "choose or unchoose a rule"},
		{"A / N", "choose every safe rule / choose none"},
		{"/", "find a rule by name"},
		{"O", "sort by size or by name"},
		{"Enter or D", "show every path of a rule"},
		{"C", "continue to the confirmation"},
		{"T", "send files to the Recycle Bin instead"},
		{"R", "scan again"},
		{"S", "save the report (on the result screen)"},
		{"? ", "this help"},
		{"Q / Esc", "go back or quit"},
	}
	for _, row := range rows {
		b.add("  " + fit(row[0], 12) + mutedStyle.Render(row[1]))
	}
	b.add("")
	b.add(mutedStyle.Render("Mouse: click a row to tick it, click a button to run it."))

	return m.renderFrame("Help", *b, mutedStyle.Render("Esc or ? close"), makeButtons(
		buttonSpec{label: "Close help", primary: true, action: actCloseHelp},
	))
}
