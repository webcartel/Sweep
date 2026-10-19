package tui

import (
	"strings"

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
	case "?", "esc", "q":
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

// viewHelp draws the keys.
func (m *Model) viewHelp() string {
	return strings.Join([]string{
		"the keys of sweep",
		"",
		"arrows / kj  move           space  rule in or out",
		"a  every safe rule in      n  every rule out",
		"o  sort by size or name    /  filter by a piece of the id",
		"enter  the items of a rule d  the items of a rule",
		"c  the confirmation        t  the bin or delete for good",
		"r  scan again              s  save the answer (at the end)",
		"q  leave                   ?  this",
		"",
		"esc or ? back",
	}, "\n")
}
