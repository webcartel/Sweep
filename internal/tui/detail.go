package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/webcartel/sweep/internal/model"
	"github.com/webcartel/sweep/internal/report"
)

const (
	screenDetail = iota + 4 // the detail of one rule
)

// detailState is what the screen of the rule remembers: the rule that is
// open and the row the cursor stands on.
type detailState struct {
	detailRule string // the id of the rule
	detailRow  int
}

// Enter of the screen of the list takes the row the cursor stands on.
func (m *Model) openDetail() (tea.Cmd, bool) {
	rows := m.visibleRows()
	if m.row < len(rows) {
		m.detailRule = ruleIDOf(rows[m.row])
		m.detailRow = 0
		m.screen = screenDetail
	}
	return nil, true
}

// detailItems answers the items of the plan of the rule that is open.
func (m *Model) detailItems() []model.Item {
	for _, rp := range m.plan.Rules {
		if ruleIDOf(rp) == m.detailRule {
			return rp.Items
		}
	}
	return nil
}

// updateDetail, the part of the screen of the rule.
func (m *Model) updateDetail(msg tea.Msg) (tea.Cmd, bool) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, false
	}
	items := m.detailItems()
	switch key.String() {
	case "up", "k":
		if m.detailRow > 0 {
			m.detailRow--
		}
	case "down", "j":
		if m.detailRow < len(items)-1 {
			m.detailRow++
		}
	case " ":
		if m.detailRow < len(items) {
			path := items[m.detailRow].Path
			m.selection.Exclude[path] = !m.selection.Exclude[path]
		}
	case "esc":
		m.screen = screenList
		m.row = 0
		for i, rp := range m.visibleRows() {
			if ruleIDOf(rp) == m.detailRule {
				m.row = i
			}
		}
	default:
		return nil, false
	}
	return nil, true
}

// viewDetail draws the rule: its items, one a line, with the mark of what
// the run keeps out.
func (m *Model) viewDetail() string {
	items := m.detailItems()
	lines := []string{fmt.Sprintf("rule %s", m.detailRule)}
	for i, item := range items {
		pick := " "
		if i == m.detailRow {
			pick = ">"
		}
		kept := m.skin.checkbox(!m.selection.Exclude[item.Path])
		lines = append(lines, fmt.Sprintf("%s %s %-60s %8s",
			pick, kept, item.Path, report.FormatBytes(item.Size)))
	}
	lines = append(lines, "space in or out  esc back")
	return strings.Join(lines, "\n")
}
