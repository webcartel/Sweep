package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/webcartel/sweep/internal/i18n"
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

// detailRuleOf answers the plan line of the rule that is open.
func (m *Model) detailRuleOf() (model.RulePlan, bool) {
	for _, rp := range m.plan.Rules {
		if ruleIDOf(rp) == m.detailRule {
			return rp, true
		}
	}
	return model.RulePlan{}, false
}

// updateDetail, the part of the screen of the rule.
func (m *Model) updateDetail(msg tea.Msg) (tea.Cmd, bool) {
	if wheel, ok := msg.(tea.MouseMsg); ok {
		switch wheel.Button {
		case tea.MouseButtonWheelUp:
			if m.detailRow > 0 {
				m.detailRow--
			}
			return nil, true
		case tea.MouseButtonWheelDown:
			if m.detailRow < len(m.detailItems())-1 {
				m.detailRow++
			}
			return nil, true
		}
	}
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
	case "esc", "q":
		m.screen = screenList
		m.row = 0
		for i, rp := range m.visibleRows() {
			if ruleIDOf(rp) == m.detailRule {
				m.row = i
			}
		}
	case "?":
		m.openHelp()
		return nil, true
	default:
		return nil, false
	}
	return nil, true
}

// viewDetail draws one rule in plain words: what it removes, how much, and
// every path the run would touch.
func (m *Model) viewDetail() string {
	rp, _ := m.detailRuleOf()
	title := m.detailRule
	about := ""
	if rp.Rule != nil {
		if rp.Rule.Name != "" {
			title = rp.Rule.Name
		}
		about = rp.Rule.Description
	}

	b := &body{}
	b.add(titleStyle.Render(title))
	b.add(mutedStyle.Render(m.detailRule))
	if about != "" {
		for _, line := range wrapText(about, m.screenWidth()-8) {
			b.add(line)
		}
	}
	b.add("")
	b.add(fmt.Sprintf(i18n.T("tui.detail.frees"),
		report.FormatBytes(rp.Bytes), humanCount(int64(rp.Files))))
	if rp.Rule != nil && rp.Rule.RequiresAdmin {
		b.add(warningStyle().Render(i18n.T("tui.detail.admin")))
	}
	if rp.Rule != nil && len(rp.Rule.RequiresClosed) != 0 {
		b.add(warningStyle().Render(i18n.T("tui.detail.closed") + strings.Join(rp.Rule.RequiresClosed, ", ")))
	}
	b.add("")

	items := m.detailItems()
	if len(items) == 0 {
		b.add(mutedStyle.Render(i18n.T("tui.detail.command")))
		if rp.Rule != nil {
			for _, target := range rp.Rule.Targets {
				if target.Kind == model.KindCommand {
					b.add("  $ " + target.Cmd + " " + strings.Join(target.Args, " "))
				}
			}
		}
	} else {
		b.add(mutedStyle.Render(i18n.T("tui.detail.keep")))
		for i, item := range items {
			mark := m.skin.checkbox(!m.selection.Exclude[item.Path])
			pick := "  "
			if i == m.detailRow {
				pick = m.skin.pointer() + " "
			}
			path := item.Path
			room := m.screenWidth() - 34
			if room < 20 {
				room = 20
			}
			if len(path) > room {
				path = m.skin.ellipsis() + path[len(path)-room:]
			}
			line := fmt.Sprintf("%s%s %-*s %9s", pick, mark, room, path, report.FormatBytes(item.Size))
			index := b.add(line)
			b.hit(index, 0, m.screenWidth()-8, actToggleItem, item.Path)
		}
	}

	return m.renderFrame(i18n.T("tui.detail.step"), *b,
		mutedStyle.Render(i18n.T("tui.detail.hint")+m.skin.sep()+i18n.T("tui.hint.back")), makeButtons(
			buttonSpec{label: i18n.T("tui.detail.backto"), primary: true, action: actBack},
			buttonSpec{label: i18n.T("tui.profile.help"), action: actHelp},
		))
}

// warningStyle colors a line that asks the reader to think.
func warningStyle() lipgloss.Style {
	return moderateStyle
}
